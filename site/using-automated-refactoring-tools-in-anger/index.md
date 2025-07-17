# Using Automated Refactoring Tools in Anger
:published: 2025-07-17
:summary: In which I resort to refactoring tools for code generation to manually implement \
          template specialization for Go and reduce run times by -40%.

## Background

Go supports functions as a first class concept and it's pretty common to pass a function to inject
custom behavior. This can have a performance impact.

In the ideal case, like in the example below, the compiler can inline everything, resulting in
optimal performance:

<!--#include-snippet file="01_background/example_test.go" display="example_test.go" -->

The loop in `apply` is inlined into `Example`, as is the function we're passing in as a parameter.
The resulting code in the loop is free of function calls (no `CALL` instruction):

```
...
    MOVD    ZR, R3
    JMP     loop_cond
loop:
    MOVD    (R0)(R3<<3), R4
    ADD     R4>>63, R4, R5
    AND     $-2, R5, R5
    SUB     R5, R4, R4
    ADD     $1, R3, R5
    NOP
    MOVD    R4, (R0)(R3<<3)
    MOVD    R5, R3
loop_cond:
    CMP     $10, R3
    BLT     loop
...
```

However, in many cases, where the function that's called can't be inlined because it's too complex
(or if we disallow the compiler to inline) Go needs to make an indirect function call from within
the loop.

Let's use `//go:noinline` to forbid the compiler from inlining `apply` and compare the generated
assembly code:

<!--#include-diff
    a="01_background/example_test.go"
    b="02_background-noinline/example_test.go"
    display="example_test.go"
-->

In this case, the loop is not inlined into `Example` and we clearly see an indirect function call.
The call `CALL` instruction is the function call, and it's indirect because it's using a register
(`R1`) as an argument instead of a label.

```
...
    MOVD    R3, fn+24(FP)
    MOVD    R1, v+8(FP)
    MOVD    R0, v(FP)
    PCDATA  $3, $2
    MOVD    ZR, R2
    JMP     loop_cond:
loop:
    MOVD    R2, i-8(SP)
    MOVD    (R0)(R2<<3), R0
    MOVD    (R3), R1
    MOVD    R3, R26
    PCDATA  $1, $0
    CALL    (R1)
    MOVD    i-8(SP), R1
    MOVD    v(FP), R2
    MOVD    R0, (R2)(R1<<3)
    ADD     $1, R1, R1
    MOVD    R2, R0
    MOVD    fn+24(FP), R3
    MOVD    R1, R2
    MOVD    v+8(FP), R1
loop_cond:
    CMP     R2, R1
    BGT     loop
...
```

In many cases, the difference between the two is unnoticeable. It would definitely not be the right
decision to always inline this function, and if the function is truly hot, Go may still inline it
when using [profile guided optimization](https://go.dev/doc/pgo).

## The Problem

I ran into this while investigating optimization opportunities for
[znkr.io/diff](https://znkr.io/diff). I had started out with a generic implementation of the diffing
algorithm that works for any type, using a signature like:

```go
func Diff[T any](x, y []T, eq  func(a, b T) bool) Edits[T]
```

However, the implementation of diffing algorithms is rather complex and so the function was never
inlined. After a number of rounds of optimizations, I [ended
up](https://github.com/znkr/diff/commit/37b4470eeb45867adcae1581907770041326e1b5) with a changed API
and an implementation that collapsed all `comparable` diffs to using the algorithm on an `int`
type[^1].

```go
func Diff[T comparable](x, y []T) Edits[T]
func DiffFunc[T any](x, y []T, eq  func(a, b T) bool) Edits[T]
```

The problem was that the `Diff` function still used an underlying algorithm for `any` and supplied
an `eq` function that was never inlined.

My hypothesis was that I could improve the performance by making sure that the `eq` function was
inlined. However, I didn't know how to do that. I tried to validate the hypothesis with a simple
hack that duplicated the implementation and specialized it by hand. To my surprise, that hack
resulted in a runtime reduction by up to
[-40%](https://github.com/znkr/diff/commit/70c8affce410b0981ab5822f46710932036564ff). So clearly,
that's a worthwhile optimization!

### Demo

Unfortunately, the diff implementation is a bit too complicated to be a good example for this blog
post. Instead, let's use a very simple (but inefficient) Quick Sort implementation (which
incidentally has a similar recursive structure as the diff implementation I am using):

<!--#include-snippet file="03_problem/sort.go" display="sort.go" -->

Manually specializing this function for `int` is straightforward:

<!--#include-diff
    a="03_problem/sort.go"
    b="03_problem/sort_int.go"
    display="sort_int.go"
-->

Comparing the runtime of these two algorithms using a simple benchmark (sorting slices with 100
random numbers) on my M1 MacBook Pro also shows a runtime improvement of -40%:

```
BenchmarkSortInt-10       435818              2658 ns/op
BenchmarkSort-10          272185              4382 ns/op
```

## The "Solution"

I tried a number of ideas but found no way to ensure the passed-in function was inlined. The
alternative of maintaining two copies of the same algorithm didn't sound very appealing. It felt
like I had to either reduce the API surface by removing the `DiffFunc` option, take a performance
hit to have both, or maintain two versions of the same algorithm. I really wished for specialization
that would allow me to write the algorithm once and change aspects of it for `comparable` types.

I liked none of these options, and I was despairing about which one to pick when it hit me: *I
could implement specialization myself!*

Go has excellent support for refactoring Go code thanks to the [go/ast](http://pkg.go.dev/go/ast)
package. We can use that to build a code generator that performs the manual specialization described
above automatically:

<!--#include-snippet file="04_solution/specialize/main.go" display="specialize/main.go" -->

This is overkill for a simple function like `Sort`, but keep in mind that this is a simple example
by construction. The same principle can be applied to much more complicated functions or even, as
in the case that triggered me to do this, whole types with multiple functions.

It's also fairly trivial to hook the specialization into a unit test to validate that the specialized
version matches the output of the generator and use `//go:generate` to regenerate the specialized
version.

The full version of what I ended up using for the diffing algorithm is
[here](https://github.com/znkr/diff/blob/main/internal/cmd/specializemyers/specialize.go).

## Conclusion

I found a way to specialize functionality in a way that doesn't require manual maintenance. Is it a
good idea, though? I don't really know, but I can drop the specialized version for a performance hit
with only a few lines of code. If this turns out to be a bad idea, or if I or someone else finds a
better solution, it's quite easily removed or replaced.

[^1]: The details don't matter here, but the idea is to assign a unique integer to every line in
      both inputs. This happens almost naturally when reducing the problem size by removing every
      line unique to either input. Both together significantly speed up a number of diffs.