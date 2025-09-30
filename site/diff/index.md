# Diff Algorithms
:published: 2025-09-30
:summary: How I overcame copying and modifying my own diff library from project to project by \
          diving too deep into diff algorithms and coming out at the other end with new diff \
          library in my hand.

For software engineers, diffs are a ubiquitous method for representing changes: We use diffs to
compare different versions of the same file (e.g., during code review or when trying to understand
the history of a file), to visualize the difference of a failing test compared with its
expectation, or to apply changes to source files automatically.

Every project I worked on professionally or privately eventually needed a diff to visualize a change
or to apply a patch. However, I have never been satisfied with any of the freely available diff
libraries. This was never really a problem professionally, but for private projects, I have copied
and modified my own library from project to project until I mentioned this to a colleague who set me
on the path to publish my Go library (a port of a previous C++ library I used to copy and modify).
*Boy, did I underestimate how close my library was to publishability!*

Anyway, I did it and I learned a whole lot about diff algorithms. You can find my library at
[znkr.io/diff](https://znkr.io/diff) and what I learned in this article. I am not finished learning
yet, so I plan to update this article as my understanding continues to evolve.

## Existing Diff Libraries

Let me start by explaining why I am dissatisfied with existing diff libraries. There are a number of
attributes that are important to me. Not all of these attributes are important for every use case,
but a diff library that I can use for all of my use cases needs to fulfill all of them.

Usually, the input to a diff algorithm is text, and most diff libraries only support that. However,
I occasionally have use cases where I need to compare things that are not text. So any diff library
that only supports text doesn't meet my needs; instead, I need support for **arbitrary sequences**.

The resulting diff output is intended to be readable by humans. Quite often, especially for text, a
good way to present a diff is in the **unified format**. However, it's not always the best
presentation. A diff library should make it easy to output a diff in unified format, but it should
also provide a way to customize the presentation by providing a **structured result**.

Besides the presentation, the content of a diff should make it easy for humans to understand the
diff. This is a somewhat subjective criterion, but there are a number of failure cases that are
easily avoided, and there's some research into **diff readability** to set a benchmark. On the other
hand, diffs should be **minimal** in that they should be as small as possible.

Last but not least, it's important that a diff library has a **simple API** and provides good
**performance** in both runtime and memory usage, even in worst-case
scenarios[^why-worst-case-scenarios].

With that, we can evaluate existing diff libraries. For Go, I went through a number of libraries
and summarized them.

| Name | Input | Output | API | Performance[^see-benchmarks] | Diff<br>Readability | Diff<br>Minimality[^see-benchmarks] |
| ---- | ----- | ------ | --- | ---------------------------- | ------------------- | ----------------------------------- |
| [diffmatchpatch](https://github.com/sergi/go-diff) | ❌[^no-arbitrary-seq] | ❌[^no-unified] | 🤔[^dmp-api] | ➖➖ | ➖ | ➖ |
| [go-internal](https://github.com/rogpeppe/go-internal/tree/master/diff)| ❌[^no-arbitrary-seq] | ❌[^no-structured-out] | 😁 | ➕➕ | ➕➕ | ➕ |
| [godebug](https://github.com/kylelemons/godebug/tree/master/diff)| ❌[^no-arbitrary-seq] |  ✅ | 😁 | ➖➖➖ /🧨[^godebug-quadratic-mem] | ➕ | ➕➕ |
| [mb0](https://github.com/mb0/diff)| ✅ |  ❌[^no-unified] | 😐[^mb0-api] | ➖➖ | ➕ | ➕➕ |
| [udiff](https://github.com/aymanbagabas/go-udiff) | ❌[^no-arbitrary-seq]| ✅ | 😁 | ➕[^udiff-threshold] | ➖ | ➖➖[^udiff-threshold]  |

NOTE[Beware]: The way I assigned ➕ and ➖ in this table doesn't follow any scientific methodology
it's merely based on running a few benchmarks and comparing a few results by hand. If you're looking
for a diff library to fulfill your needs, I would like to encourage you to do your own comparisons.
You can find the code I used for these comparisons in [on
github](https://github.com/znkr/diff/tree/main/internal/benchmarks).

[^why-worst-case-scenarios]: Here is one real-world example of why worst-case scenarios are
    important: Imagine you're breaking an existing feature in a way that triggers a worst-case
    scenario in a test. If the test is running for a very long time or runs out of memory, you're
    going to have to debug two problems instead of one.
[^see-benchmarks]: See [benchmark_comparison.txt](/diff/benchmark_comparison.txt)
    for the source of these ratings.
[^no-arbitrary-seq]: No support for arbitrary sequences
[^no-unified]: No support for unified diffs
[^no-structured-out]: No support for structured results
[^dmp-api]: The diffmatchpatch API is very hard to use
[^mb0-api]: The mb0 API is from before generics and is a bit cumbersome to use
[^godebug-quadratic-mem]: Quadratic memory use; for my test cases, this resulted in >30 GB of memory
    used.
[^udiff-threshold]: udiff has a very low threshold for when it starts to stop searching for an
optimal solution. This improves the speed, but it also results in relatively large diffs.

## Challenges

The results suggest that it's far from trivial to implement a good diff library, and the one I had
started out with wasn't much better. To understand why the existing libraries are as they are,
we need to take a peek into the implementation.

### Complexity

With the exception of go-internal, all libraries use [Myers'
Algorithm](http://www.xmailserver.org/diff2.pdf) to compute the diff. This is a standard algorithm
that returns a minimal diff and has been in use for this purpose for decades. The algorithm has a
runtime complexity of $\mathcal{O}(ND)$ where $N$ is the number of input elements and $D$ is the
edit distance between the two inputs. This means that the algorithm is very fast for inputs that are
similar, which is quite common. However, it's essentially quadratic in the worst case. That is, for
inputs that are very different, the complexity approaches $\mathcal{O}(N^2)$. Furthermore, the
algorithm comes in two variants with a space complexity of either $\mathcal{O}(N^2)$ or
$\mathcal{O}(N)$. Only godebug uses the variant with quadratic memory growth.

This means that **it's relatively easy to write a well-performing diffing algorithm for small or
similar inputs, but it takes a very long time to complete for larger, less similar inputs**. A
consequence of this is that we can't trust simple benchmarks; instead, we need to test the
worst-case scenario[^why-worst-case-scenarios].

As always in cases like this, we can improve the performance by approximating an optimal solution.
There are a number of heuristics that reduce the time complexity by trading off diff minimality. For
example, diffmatchpatch uses a deadline to stop the search for an optimal diff, and udiff uses a
an extremely aggressive heuristic.

Instead of improving Myers' runtime with heuristics, it's also often possible to find a diff using
only heuristics. go-internal uses [patience diff](https://bramcohen.livejournal.com/73318.html). The
heuristic is good enough that it alone almost always results in a good diff with a runtime
complexity of $\mathcal{O}(N \, \log \, N)$[^not-all-patience-diffs]. An additional advantage of
this algorithm is that it produces more readable diffs. However, patience diff can fail with very
large diffs, and it can only be implemented efficiently using a hash table, which restricts the
possible applications.

TIP[Histogram Diff]: Besides patience diff, there's another interesting heuristic called histogram
diff. I still have to implement it and understand it better before writing about it here, though.

[^not-all-patience-diffs]: There's no single patience diff heuristic, instead there are different
    implementations with different performance characteristics.

### Readability

Diff algorithms usually find a minimal diff or an approximation of one. However, except for trivial
cases, there are always multiple minimal diffs. For example, this simple diff

<!--#include-diff diff="example_01.diff" -->

is as minimal as

<!--#include-diff diff="example_02.diff" -->
 
Not all of the minimal or near-minimal diffs have the same readability for humans. For
example[^the-if-works-linear-meyers],

<!--#include-diff diff="example_03.diff" -->

is much more readable than the equally minimal and correct

<!--#include-diff diff="example_04.diff" -->

Furthermore, if we relax minimality to accept approximations, the number of possible results
increases significantly.

For good diff readability, we have to select one solution from the many possible ones that is
readable for humans. Many people believe that the diff readability is determined by the algorithm.
However, that's only partially correct, because **different *implementations* of the same algorithm
can produce vastly different results**.

There's also been a lot of progress in the past years to improve diff readability. Perhaps the best
work about diff readability is [diff-slider-tools](https://github.com/mhagger/diff-slider-tools) by
[Michael Haggerty](https://github.com/mhagger). He implemented a heuristic that's applied in a
post-processing step to improve the readability.

In fact, `example_03.diff` above was generated using this heuristic. The diff without the heuristic,
as generated by my implementation of Myers' linear-space variant, looks like this:

<!--#include-diff diff="example_03_no_indent_heuristic.diff" -->

Notice that the deletion starts at the end of the preceding function and leaves a small
remainder of the function being deleted? Michael's heuristic fixes this problem and results in the
very readable `example_03.diff`.

NOTE[It's not the algorithm]: `example_04.diff` was found using a different implementation of Myers'
linear-space variant. That is, both `example_03.diff` and `example_04.diff` used the same algorithm!
The differences stem from the implementation of that algorithm and from post-processing.

[^the-if-works-linear-meyers]: Stolen from
    [https://blog.jcoglan.com/2017/03/22/myers-diff-in-linear-space-theory/](https://blog.jcoglan.com/2017/03/22/myers-diff-in-linear-space-theory/)

## A New Diffing Library for Go

I created [znkr.io/diff](https://znkr.io/diff) to address these challenges in a way that works for
all my use cases. Let's reiterate what I want from a diffing library:

*  The input can be text and arbitrary slices
*  The output should be possible in unified format and as a structured result
*  The API should be simple
*  The diffs should be minimal or near-minimal
*  The runtime and memory performance should be excellent

This is a lot more than what any of the existing libraries provide. When I copied and modified my
old diffing library, I could adapt it to the use cases at hand. But a general-purpose diffing
library needs to be general enough to cover the vast majority of use cases. At the same time, it
needs to be extensible to make sure new features can be implemented without cluttering the API over
time.

Unfortunately, excellent performance and minimal results are somewhat in opposition to one another
and I ended up providing three different modes of operation: Default (balanced between performance
and minimality), Fast (sacrifice minimal results for faster speed), Optimal (minimal result whatever
the cost).

| Mode | Input | Output | API | Performance[^see-benchmarks] | Diff<br>Readability | Diff<br>Minimality[^see-benchmarks] |
| ---- | ----- | ------ | --- | ---------------------------- | ------------------- | ----------------------------------- |
| Default | ✅ | ✅ | 😁 | ➕➕ | ➕➕ | ➕➕ |
| Fast | ✅ | ✅ | 😁 | ➕➕➕ | ➕➕ | ➕ |
| Optimal | ✅ | ✅ | 😁 | ➕ | ➕➕ | ➕➕ |

NOTE[Text Only]: This table only applies to text (same as the table above), non-text inputs can have
a different performance (if they are not `comparable` or readability).

### API

To design this API, I started with the data structures that I wanted to use as a user of the API and
worked backwards from there. At a very high level, there are two structured representations of a
diff that have been useful to me: a flat sequence of all deletions, insertions, and matching
elements (called *edits*) and a nested sequence of consecutive changes (called *hunks*).

*  Edits are what I use to represent edits in this article; they contain the full content of both
   inputs and how one is transformed into the other.
*  Hunks are a great representation for unit tests, because they are empty if both inputs are
   identical and they make it possible to visualize just the changes even if the inputs are large.

#### Arbitrary Slices

I started with the design for the most general case, arbitrary slices. The Go representation for
diffing slices I liked the most is this one (see also
[znkr.io/diff](https://pkg.go.dev/znkr.io/diff)):

<!--#include-snippet file="diff.go" lines="5..28" -->

The alternatives I have seen are variations and combinations of two themes. Either using slices to
represent edit operations in `Hunk`

```go
type Hunk[T any] struct {
	Delete []T
	Insert []T
	Match  []T
}
```

Or using indices instead of elements

```go
type Edit struct {
	Op         Op
	PosX, PosY []int
}
```

All of these representations work, but I found that the representations above served my use cases
best. One little quirk is that `Edit` always contains both elements. This is often unnecessary, but
there are use cases where this is very important because the elements themselves might not be equal
(e.g., if they are pointers that are compared with a custom function).

Once the data structures were established, it was quite obvious that the simplest way to fill them
with diff data was to write two functions [`diff.Edits`](https://pkg.go.dev/znkr.io/diff#Edits) and
[`diff.Hunks`](https://pkg.go.dev/znkr.io/diff#Hunks) to return the diffs. I made them extensible by
using [functional options](https://dave.cheney.net/2014/10/17/functional-options-for-friendly-apis).

<!--#include-snippet file="diff.go" lines="30..42" -->

The options allow for future extensibility and allow changing the behavior of these functions. For
example, the option [`diff.Context(5)`](https://pkg.go.dev/znkr.io/diff#Context) configures `Hunks`
to provide 5 elements of surrounding context.

However, the current API still doesn't allow *arbitrary slices*; it only allows slices of
`comparable` types. To fix this, I needed two other functions that provide a function to compare
two elements. The Go standard library uses the `Func` suffix for functions like this, so I followed
the lead:

<!--#include-snippet file="diff.go" lines="44..50" -->

#### Text

While this API works well to produce a structured result for arbitrary slices, it doesn't provide
output in unified format for text inputs. My first approach was to provide a helper function that
returns a diff in unified format: `diff.ToUnified(hunks []Hunk[string]) string`. However, this would
make getting a unified diff more complicated. Besides requiring two function calls, it would be
necessary to split the input into lines. This, in turn, can be done in different ways, e.g., by
stripping or keeping the line breaks, which opens the door to mistakes. It's much better to provide
a simple function for the entire use case.

<!--#include-snippet file="textdiff.go" lines="7..9" -->

I also moved this function to the [`textdiff`](https://pkg.go.dev/znkr.io/diff/textdiff) package to
highlight the difference in expected input.

Now, I also happen to have use cases where I need structured results for text diffs. It would be
very annoying if I had to split those into lines manually. Besides, I can make a few more
assumptions about text that allow for a slight simplification of the data structures:

<!--#include-snippet file="textdiff.go" lines="11..30" -->

#### Conclusion

For the full API and examples for how to use it, please see the package documentation for
[znkr.io/diff](https://pkg.go.dev/znkr.io/diff) and
[znkr.io/diff/textdiff](https://pkg.go.dev/znkr.io/diff/textdiff). I am certain that there are use
cases not covered by this API, but I feel confident that it can evolve to cover these use cases in
the future. For now, all my needs are fulfilled, but if you run into a situation that can't be
solved by this API or requires some contortions, please [tell me about
it](https://github.com/znkr/diff/issues/new).

### Implementation

To implement this API, we need to implement a diff algorithm. There are a couple of standard diff
algorithms that we can choose from. The choice of the algorithm as well as how it's implemented
matters for the readability of the result as well as the performance.

A good starting point for this project was Myers' algorithm, simply because it's the fastest
algorithm that can cover the whole API. In particular, the `...Func` variants for `any` types
instead of `comparable` can't make use of a hash map. Patience and Histogram require the use of a
hash map for an efficient implementation, so Myers' really is the only choice. Another advantage of
Myers' compared to Patience and Histogram is that it will return optimal results.

On the flip side, in the [comparison above](#existing-diff-libraries), it came out as relatively
slow compared to the patience diff algorithm and didn't produce the most readable results. It turns
out, however, that this can be mitigated and almost completely overcome for `comparable` types using
a combination of preprocessing, heuristics, and post-processing.

I am not going to cover the diff algorithm in detail here. There are a number of excellent articles
on the web that describe it[^myers-algorithm-articles], but I recommend reading the
paper[^myers-algorithm-paper]: All articles I have seen try to keep a distance from the theory that
makes this algorithm work, but that's not really helpful if you want to understand how and why this
algorithm works.

[^myers-algorithm-articles]: I can recommend
    [https://blog.robertelder.org/diff-algorithm/](https://blog.robertelder.org/diff-algorithm/) and
    this 5 part series
    [https://blog.jcoglan.com/2017/02/12/the-myers-diff-algorithm-part-1/](https://blog.jcoglan.com/2017/02/12/the-myers-diff-algorithm-part-1/)
[^myers-algorithm-paper]: Myers, E.W. An O(ND) difference algorithm and its variations. Algorithmica
    1, 251-266 (1986). [https://doi.org/10.1007/BF01840446](https://doi.org/10.1007/BF01840446)

#### Preprocessing

The most impactful way to improve the performance of Myers' algorithm is to reduce the problem size.
The simplest thing to do is to strip any common prefix and suffix. This is always possible and helps
a little. However, it can also reduce diff readability, because it will consume matching elements
eagerly.

For example, let's say we have this change:

<!--#include-diff diff="example_05.diff" -->

If we eagerly consume the common prefix first and then the common suffix, the first 11 lines are
all identical and the so are the last 4. This in turn would result in a different diff:

<!--#include-diff diff="example_05_strip_common_prefix_and_suffix.diff" -->

Fortunately, this is easy to fix in post processing.

Much more impactful, but only efficiently possible for `comparable` types, is to remove all elements
that are unique to either the left side or the right side, as those must always be deletions or
insertions. Non-`comparable` types can't be keys in a hash map in Go, which is necessary for
checking uniqueness. This preprocessing step [reduced the runtime by up to
99%](https://github.com/znkr/diff/commit/37b4470eeb45867adcae1581907770041326e1b5) for a few
real-world worst-case diffs.

In contrast to the suffix and prefix removal, stripping unique elements doesn't have any readability
impact.

#### Heuristics

Another very impactful way to improve the performance is *Anchoring*. It is based on [patience
diff](https://bramcohen.livejournal.com/73318.html). The word patience is a bit misleading, because
it's too easily associated with having to wait and it doesn't describe the heuristic very well
either. It works by finding elements that are occur exactly once on both the left and the right
side. When we matching up these unique pairs we create a segmentation of the input into smaller
parts that can be analyzed individually. Even better, we're very likely to find matching lines atop
and below such a pair of unique elements. This allows us to shrink the segments by stripping common
prefixes and suffixes. This heuristic [reduced the runtime by up to
95%](https://github.com/znkr/diff/commit/feb7bda337f269935d80ee18e703e0940f406873). Unfortunately,
finding unique elements and matching them up requires a hash map again which means that it can only
be used for `comparable` types.

There are two more heuristics that are I implemented. They help for non-`comparable` types and as a
backstop when the other heuristics don't work. Their main purpose is to avoid runaway quadratic
growth. The *Good Diagonal* heuristic stops searching for a better solution if we found a solution
that's good enough and the *Too Expensive* heuristic shortcuts the search if it becomes too
expensive which reduces the worst-case complexity from $\mathcal{O}(N^2)$ to
$\mathcal{O}(N^1.5 \, \log \, N)$.

However, heuristics like this trade diff minimality for performance, this is not always desirable.
Sometimes, a minimal diff is exactly what's required.
[`diff.Optimal`](https://pkg.go.dev/znkr.io/diff#Optimal) disables these heuristics to always find a
minimal diff irrespective of the costs.

#### Post-processing

We established before that a diff algorithm finds one of many possible solutions. Given such a
solution we can discover more solutions by it locally and then selecting the best solution according
to some metric. This is exactly how [Michael Haggerty's](https://github.com/mhagger) indentation
heuristic works for text. 

For any given diff, we can often slide the edits up or down in a way that doesn't change the meaning
of a diff. For example,

<!--#include-diff diff="example_06.diff" -->

has the same meaning as

<!--#include-diff diff="example_06_indent_heuristic.diff" -->

We call edits that can be slid up or down *sliders*. The question is, how do we select the best
slide? Michael collected human ratings for different sliders of the same diff and used them to
develop a heuristic to match these ratings:
[diff-slider-tools](https://github.com/mhagger/diff-slider-tools).

However, this heuristic only works for text and is tuned towards code instead of prose. I decided to
make it optional. It can be enabled with the
[`textdiff.IndentHeuristic`](https://pkg.go.dev/znkr.io/diff/textdiff#IndentHeuristic) option.

#### Diff Representation

The representation used during the execution of the diff algorithm has a surprising impact on the
algorithm performance and result readability. This is not at all obvious, and so it took me a while
to figure out that the best approach is akin to a side-by-side view of a diff: You use two `[]bool`
slices to represent the left side and the right side respectively: `true` in the left side slice 
represents a deletion and on the right side an insertion. `false` is a matching element.

This representation has four big advantages: It can be preallocated, the order in which edits are
discovered doesn't matter, it's easy to mutate during post-processing, and it's easy to generate
other representations from it.

## Open Questions

* What exactly is the reason that two different algorithms produce different results? - I looked
  into this question a little, but I haven't found a conclusive answer yet.

## Conclusion

Diff algorithms are relatively complicated by themselves, but they pale in comparison  to what's
necessary to provide a high-quality diff library. This article tries to explain what went into
my new diff library, but there's still more that I haven't implemented yet.