package build

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func upper(s string) (string, error) { return strings.ToUpper(s), nil }

// counted returns a rule that reports how many times it has run.
func counted[T, R any](fn func(T) R) (func(T) (R, error), *int) {
	n := 0
	return func(v T) (R, error) {
		n++
		return fn(v), nil
	}, &n
}

func get[T any](t *testing.T, c *Cache, a Artifact[T]) T {
	t.Helper()
	v, err := a.Get(t.Context(), c)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	return v
}

// unbounded returns a cache that evicts nothing, which is what a test wants
// unless it is about eviction.
func unbounded() *Cache { return NewCache(0) }

// plan returns what [Collect] takes: an artifact holding one artifact per
// element of in, each computed by rule under kind. It stands for the step whose
// units of work are only known once it has run.
func plan(kind string, rule any, in []string) Artifact[[]Artifact[string]] {
	return Derive[[]Artifact[string]](kind+" plan", func(elems []string) ([]Artifact[string], error) {
		out := make([]Artifact[string], len(elems))
		for i, el := range elems {
			out[i] = Derive[string](kind, rule, el)
		}
		return out, nil
	}, Const(kind+" src", in))
}

// values returns the values of a collection, sorted, so that a test can assert
// on what was computed without depending on the keys it was held under.
func values[T cmp.Ordered](m map[Key]T) []T {
	out := make([]T, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	slices.Sort(out)
	return out
}

func TestDeriveComputesOncePerKey(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	a := Derive[string]("upper", rule, "hello")
	b := Derive[string]("upper", rule, "hello")

	if a.Key() != b.Key() {
		t.Error("equal arguments produced different keys")
	}
	if got := get(t, c, a); got != "HELLO" {
		t.Errorf("Get() = %q, want %q", got, "HELLO")
	}
	if got := get(t, c, b); got != "HELLO" {
		t.Errorf("Get() = %q, want %q", got, "HELLO")
	}
	if *runs != 1 {
		t.Errorf("rule ran %d times, want 1", *runs)
	}
}

// TestDeclaringTouchesNothing checks the property the design rests on:
// declaring an artifact runs no rule and stores nothing, so it needs no cache.
func TestDeclaringTouchesNothing(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	Derive[string]("upper", rule, "hello")
	Collect[string]("uppers", plan("upper", rule, []string{"x"}))

	if *runs != 0 {
		t.Errorf("declaring ran the rule %d times, want 0", *runs)
	}
	if c.Len() != 0 {
		t.Errorf("declaring put %d values in the cache, want 0", c.Len())
	}
}

func TestChangedParameterReruns(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	get(t, c, Derive[string]("upper", rule, "a"))
	get(t, c, Derive[string]("upper", rule, "b"))

	if *runs != 2 {
		t.Errorf("rule ran %d times, want 2", *runs)
	}
}

func TestChangedDependencyReruns(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	for _, in := range []string{"a", "b"} {
		get(t, c, Derive[string]("upper", rule, Const("src", in)))
	}
	if *runs != 2 {
		t.Errorf("rule ran %d times, want 2", *runs)
	}

	// The same dependency twice is one artifact.
	get(t, c, Derive[string]("upper", rule, Const("src", "a")))
	if *runs != 2 {
		t.Errorf("rule ran %d times after repeating an input, want 2", *runs)
	}
}

func TestDependencyValueIsPassedNotTheArtifact(t *testing.T) {
	c := unbounded()
	src := Const("src", "world")
	greet := func(who string) (string, error) { return "hello " + who, nil }

	if got := get(t, c, Derive[string]("greet", greet, src)); got != "hello world" {
		t.Errorf("Get() = %q, want %q", got, "hello world")
	}
}

func TestSliceOfDependencies(t *testing.T) {
	c := unbounded()
	var parts []Artifact[string]
	for _, s := range []string{"a", "b", "c"} {
		parts = append(parts, Const("src", s))
	}
	join := func(ss []string, sep string) (string, error) { return strings.Join(ss, sep), nil }

	if got := get(t, c, Derive[string]("join", join, parts, "-")); got != "a-b-c" {
		t.Errorf("Get() = %q, want %q", got, "a-b-c")
	}
}

func TestErrorFromARuleReachesTheCaller(t *testing.T) {
	c := unbounded()
	boom := errors.New("boom")
	fail := func(string) (string, error) { return "", boom }

	a := Derive[string]("fail", fail, "x")
	if _, err := a.Get(t.Context(), c); !errors.Is(err, boom) {
		t.Errorf("Get() error = %v, want %v", err, boom)
	}

	// A rule that fails is not run again, and the failure reaches whatever
	// depends on it.
	b := Derive[string]("upper", upper, a)
	if _, err := b.Get(t.Context(), c); !errors.Is(err, boom) {
		t.Errorf("dependent Get() error = %v, want %v", err, boom)
	}
}

func TestGetIsSafeUnderConcurrentCallers(t *testing.T) {
	c := unbounded()
	var mu sync.Mutex
	runs := 0
	rule := func(s string) (string, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		return strings.ToUpper(s), nil
	}

	a := Derive[string]("upper", rule, "hello")
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := a.Get(t.Context(), c); err != nil {
				t.Errorf("Get() = %v", err)
			}
		}()
	}
	wg.Wait()

	if runs != 1 {
		t.Errorf("rule ran %d times, want 1", runs)
	}
}

// --- context

func TestRulesAreGivenTheCallersContext(t *testing.T) {
	c := unbounded()
	type key struct{}
	rule := func(ctx context.Context, s string) (string, error) {
		tag, _ := ctx.Value(key{}).(string)
		return tag + s, nil
	}
	ctx := context.WithValue(t.Context(), key{}, "tagged:")

	got, err := Derive[string]("tag", rule, "a").Get(ctx, c)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if want := "tagged:a"; got != want {
		t.Errorf("Get() = %q, want %q", got, want)
	}

	// The artifacts under a collection are given it too.
	got2, err := Collect[string]("tags", plan("tag", rule, []string{"b", "c"})).Get(ctx, c)
	if err != nil {
		t.Fatalf("Get() = %v", err)
	}
	if want := []string{"tagged:b", "tagged:c"}; !slices.Equal(values(got2), want) {
		t.Errorf("Get() = %q, want %q", values(got2), want)
	}
}

// TestACanceledGetRunsNothing checks the cheapest half of cancellation: a
// caller that has already gone away starts no work.
func TestACanceledGetRunsNothing(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := Derive[string]("upper", rule, "hello").Get(ctx, c); !errors.Is(err, context.Canceled) {
		t.Errorf("Get() error = %v, want %v", err, context.Canceled)
	}
	if *runs != 0 {
		t.Errorf("rule ran %d times, want 0", *runs)
	}
	if c.Len() != 0 {
		t.Errorf("cache holds %d values, want 0", c.Len())
	}
}

// TestACancellationIsNotCached is the other half: a rule that stopped because
// its caller went away produced no value, so the next caller computes it again
// rather than being handed the cancellation.
func TestACancellationIsNotCached(t *testing.T) {
	c := unbounded()
	ctx, cancel := context.WithCancel(t.Context())

	runs := 0
	rule := func(ctx context.Context, s string) (string, error) {
		runs++
		if runs == 1 {
			cancel()
			return "", ctx.Err()
		}
		return strings.ToUpper(s), nil
	}
	a := Derive[string]("upper", rule, "hello")

	if _, err := a.Get(ctx, c); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get() error = %v, want %v", err, context.Canceled)
	}
	if c.Len() != 0 {
		t.Errorf("cache holds %d values after a cancellation, want 0", c.Len())
	}
	if got := get(t, c, a); got != "HELLO" {
		t.Errorf("Get() after a cancellation = %q, want %q", got, "HELLO")
	}
	if runs != 2 {
		t.Errorf("rule ran %d times, want 2: once canceled, once for real", runs)
	}
}

// TestACancellationIsNotCachedWhenTheErrorHidesIt checks the same property for a
// rule that reports a cancellation as an error of its own. Not every rule wraps
// the context error, so the cache consults the context as well: caching such a
// failure would serve it for the life of the cache.
func TestACancellationIsNotCachedWhenTheErrorHidesIt(t *testing.T) {
	c := unbounded()
	ctx, cancel := context.WithCancel(t.Context())

	runs := 0
	rule := func(ctx context.Context, s string) (string, error) {
		runs++
		if runs == 1 {
			cancel()
			return "", fmt.Errorf("compiling %s: %v", s, ctx.Err())
		}
		return strings.ToUpper(s), nil
	}
	a := Derive[string]("upper", rule, "hello")

	if _, err := a.Get(ctx, c); err == nil {
		t.Fatal("Get() = nil, want an error")
	}
	if c.Len() != 0 {
		t.Errorf("cache holds %d values after a cancellation, want 0", c.Len())
	}
	if got := get(t, c, a); got != "HELLO" {
		t.Errorf("Get() after a cancellation = %q, want %q", got, "HELLO")
	}
}

// TestAPanicLeavesNothingHeld checks that a rule that panics releases the entry
// it took charge of. Leaving it with a wait channel that never closes blocks
// every later lookup of the key, and eviction cannot free it either.
func TestAPanicLeavesNothingHeld(t *testing.T) {
	c := unbounded()

	runs := 0
	rule := func(s string) (string, error) {
		runs++
		if runs == 1 {
			panic("boom")
		}
		return strings.ToUpper(s), nil
	}
	a := Derive[string]("upper", rule, "hello")

	func() {
		defer func() {
			if recover() == nil {
				t.Error("Get() returned normally, want the rule's panic")
			}
		}()
		a.Get(t.Context(), c)
	}()

	if c.Len() != 0 {
		t.Errorf("cache holds %d values after a panic, want 0", c.Len())
	}

	// A second Get must compute the value rather than wait on the entry the
	// panic left behind, so the deadline is what tells the two apart.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if got, err := a.Get(ctx, c); err != nil || got != "HELLO" {
		t.Errorf("Get() after a panic = %q, %v, want %q, nil", got, err, "HELLO")
	}
}

// TestACancellationStopsOnlyTheCallerThatAskedForIt checks what the cache owes
// a caller waiting on a computation someone else started: the value, computed
// again under its own context if the first caller went away.
func TestACancellationStopsOnlyTheCallerThatAskedForIt(t *testing.T) {
	c := unbounded()
	type slow struct{}

	var mu sync.Mutex
	runs := 0
	inside := make(chan struct{})
	rule := func(ctx context.Context, s string) (string, error) {
		mu.Lock()
		runs++
		mu.Unlock()
		if ctx.Value(slow{}) != nil {
			close(inside)
			<-ctx.Done()
			return "", ctx.Err()
		}
		return strings.ToUpper(s), nil
	}
	a := Derive[string]("upper", rule, "hello")

	ctx, cancel := context.WithCancel(context.WithValue(t.Context(), slow{}, true))
	canceled := make(chan struct{})
	go func() {
		defer close(canceled)
		if _, err := a.Get(ctx, c); !errors.Is(err, context.Canceled) {
			t.Errorf("canceled Get() error = %v, want %v", err, context.Canceled)
		}
	}()

	<-inside // the first caller is in the rule, so the second one waits on it
	second := make(chan string, 1)
	go func() {
		v, err := a.Get(t.Context(), c)
		if err != nil {
			t.Errorf("Get() = %v", err)
		}
		second <- v
	}()
	cancel()

	if got := <-second; got != "HELLO" {
		t.Errorf("Get() = %q, want %q", got, "HELLO")
	}
	<-canceled
	mu.Lock()
	defer mu.Unlock()
	if runs != 2 {
		t.Errorf("rule ran %d times, want 2: once canceled, once for the caller that stayed", runs)
	}
}

// TestReuseNeedsNothingHeld is what separating the declaration from the cache
// is for. A build is declared, forced, and dropped entirely; declaring the same
// build again reuses every value, because a key stands for what its inputs
// produce and nothing has to survive for that to hold.
func TestReuseNeedsNothingHeld(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	build := func() string {
		src := Const("src", "hello")
		return get(t, c, Derive[string]("upper", rule, src))
	}

	if got := build(); got != "HELLO" {
		t.Fatalf("Get() = %q, want %q", got, "HELLO")
	}
	runtime.GC()
	if got := build(); got != "HELLO" {
		t.Fatalf("Get() = %q, want %q", got, "HELLO")
	}

	if *runs != 1 {
		t.Errorf("rule ran %d times across two builds, want 1", *runs)
	}
	if st := c.Stats(); st.Misses != 2 { // one for the const, one for upper
		t.Errorf("the two builds ran %d rules in total, want 2", st.Misses)
	}
}

func TestStats(t *testing.T) {
	c := unbounded()
	a := Derive[string]("upper", upper, "hello")
	get(t, c, a)
	get(t, c, a)
	get(t, c, Derive[string]("upper", upper, "goodbye"))

	st := c.Stats()
	if want := (Stat{Hits: 1, Misses: 2}); st.Kinds["upper"] != want {
		t.Errorf("Stats().Kinds[upper] = %+v, want %+v", st.Kinds["upper"], want)
	}
	if st.Hits != 1 || st.Misses != 2 {
		t.Errorf("Stats() = %d hits, %d misses, want 1, 2", st.Hits, st.Misses)
	}
	if st := c.Stats(); st.Hits != 0 || st.Misses != 0 {
		t.Errorf("Stats() = %+v after reading it, want empty", st)
	}
}

// --- eviction

// TestEvictionDropsTheColdest checks that the bound is enforced and that what
// goes is what has least recently been reached.
func TestEvictionDropsTheColdest(t *testing.T) {
	c := NewCache(2)

	cold := Derive[string]("upper", upper, "cold")
	warm := Derive[string]("upper", upper, "warm")
	get(t, c, cold)
	get(t, c, warm)
	get(t, c, cold) // cold is now the more recently reached of the two

	get(t, c, Derive[string]("upper", upper, "new"))

	if c.Len() > 2 {
		t.Errorf("cache holds %d values, want at most 2", c.Len())
	}
	c.Stats()
	get(t, c, warm)
	if st := c.Stats(); st.Kinds["upper"].Misses != 1 {
		t.Errorf("the coldest value survived eviction: %+v", st.Kinds["upper"])
	}
}

// TestEvictionCostsWorkNotCorrectness checks the property that lets any
// eviction policy be safe: a declaration carries what its rules need, so a
// value the cache has dropped can always be computed again.
func TestEvictionCostsWorkNotCorrectness(t *testing.T) {
	c := NewCache(2)
	rule, runs := counted(strings.ToUpper)

	src := Const("src", "hello")
	a := Derive[string]("upper", rule, src)
	if got := get(t, c, a); got != "HELLO" {
		t.Fatalf("Get() = %q, want %q", got, "HELLO")
	}

	// Push it out with unrelated work.
	for i := range 10 {
		get(t, c, Derive[string]("upper", upper, strconv.Itoa(i)))
	}

	if got := get(t, c, a); got != "HELLO" {
		t.Errorf("Get() after eviction = %q, want %q", got, "HELLO")
	}
	if *runs != 2 {
		t.Errorf("rule ran %d times, want 2: once, then again after eviction", *runs)
	}
}

// TestEvictionSparesAValueBeingComputed checks the compute-once guarantee
// under eviction. A result is stamped when its lookup starts and the clock runs
// on through everything its rule asks for, so a value still being computed is
// among the coldest by the time it is done.
func TestEvictionSparesAValueBeingComputed(t *testing.T) {
	c := NewCache(4)

	var mu sync.Mutex
	runs := 0
	inside := make(chan struct{})
	release := make(chan struct{})
	rule := func(s string) (string, error) {
		mu.Lock()
		runs++
		first := runs == 1
		mu.Unlock()
		if first {
			close(inside)
			<-release
		}
		return strings.ToUpper(s), nil
	}
	a := Derive[string]("slow", rule, "hello")

	computing := make(chan struct{})
	go func() {
		defer close(computing)
		if _, err := a.Get(t.Context(), c); err != nil {
			t.Errorf("Get() = %v", err)
		}
	}()
	<-inside // the first caller is in the rule, so the second one waits on it

	waiting := make(chan string, 1)
	go func() {
		v, err := a.Get(t.Context(), c)
		if err != nil {
			t.Errorf("Get() = %v", err)
		}
		waiting <- v
	}()

	// Unrelated work, enough to take the cache over its bound while the value
	// is still being computed.
	for i := range 12 {
		get(t, c, Derive[string]("upper", upper, strconv.Itoa(i)))
	}
	close(release)

	if got := <-waiting; got != "HELLO" {
		t.Errorf("Get() = %q, want %q", got, "HELLO")
	}
	<-computing
	mu.Lock()
	defer mu.Unlock()
	if runs != 1 {
		t.Errorf("rule ran %d times, want 1", runs)
	}
}

// TestReachingAValueReachesWhatItWasMadeOf is what keeps the artifacts nothing
// asks for directly. The artifacts under a collection are reached by computing
// it and never by a caller, so a policy that counted only direct use would drop
// them.
func TestReachingAValueReachesWhatItWasMadeOf(t *testing.T) {
	c := NewCache(8)
	rule, runs := counted(strings.ToUpper)

	all := Collect[string]("uppers", plan("upper", rule, []string{"a", "b", "c"}))
	get(t, c, all)
	if *runs != 3 {
		t.Fatalf("rule ran %d times, want 3", *runs)
	}

	// Ask only for the collection, repeatedly, while unrelated work competes
	// for the bound. The artifacts under it are never asked for, and must
	// survive anyway.
	for i := range 10 {
		get(t, c, all)
		get(t, c, Derive[string]("upper", upper, strconv.Itoa(i)))
	}

	// A collection over the same elements in a different order shares them.
	got := get(t, c, Collect[string]("uppers", plan("shuffled", rule, []string{"c", "b", "a"})))
	if want := []string{"A", "B", "C"}; !slices.Equal(values(got), want) {
		t.Errorf("Get() = %v, want %v", values(got), want)
	}
	if *runs != 3 {
		t.Errorf("rule ran %d times, want 3: the elements were dropped", *runs)
	}
}

// --- Collect

func TestCollect(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	got := get(t, c, Collect[string]("uppers", plan("upper", rule, []string{"a", "b"})))
	if want := []string{"A", "B"}; !slices.Equal(values(got), want) {
		t.Errorf("Get() = %v, want %v", values(got), want)
	}
	if *runs != 2 {
		t.Errorf("rule ran %d times, want 2", *runs)
	}
}

func TestCollectKeysByTheArtifact(t *testing.T) {
	c := unbounded()
	in := []string{"a", "b"}

	got := get(t, c, Collect[string]("uppers", plan("upper", upper, in)))
	for _, el := range in {
		a := Derive[string]("upper", upper, el)
		if v, ok := got[a.Key()]; !ok || v != strings.ToUpper(el) {
			t.Errorf("collection[%s] = %q, %v, want %q", el, v, ok, strings.ToUpper(el))
		}
	}
}

func TestCollectSharesElementsBetweenBatches(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	for i, in := range [][]string{{"a", "b"}, {"b", "c"}} {
		get(t, c, Collect[string]("uppers", plan(fmt.Sprintf("upper%d", i), rule, in)))
	}
	// a, b, c: b is shared by the two batches.
	if *runs != 3 {
		t.Errorf("rule ran %d times, want 3", *runs)
	}
}

func TestCollectHoldsARepeatedElementOnce(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	got := get(t, c, Collect[string]("uppers", plan("upper", rule, []string{"a", "b", "a"})))
	if want := []string{"A", "B"}; !slices.Equal(values(got), want) {
		t.Errorf("Get() = %v, want %v", values(got), want)
	}
	// The repeated element is one artifact, so it is computed once and held
	// under one key.
	if *runs != 2 {
		t.Errorf("rule ran %d times, want 2", *runs)
	}
}

func TestCollectFailsWithTheArtifactThatFailed(t *testing.T) {
	c := unbounded()
	boom := errors.New("boom")
	rule := func(s string) (string, error) {
		if s == "b" {
			return "", boom
		}
		return s, nil
	}

	a := Collect[string]("uppers", plan("upper", rule, []string{"a", "b"}))
	if _, err := a.Get(t.Context(), c); !errors.Is(err, boom) {
		t.Errorf("Get() error = %v, want %v", err, boom)
	}
}

// --- keys and declaration errors

func TestDerivePanics(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
		want string
	}{{
		name: "not a function",
		fn:   func() { Derive[string]("k", "not a function", "x") },
		want: "rule is string",
	}, {
		name: "wrong number of arguments",
		fn:   func() { Derive[string]("k", upper, "x", "y") },
		want: "takes 1 arguments, given 2",
	}, {
		name: "wrong number of arguments after a context",
		fn: func() {
			Derive[string]("k", func(context.Context, string) (string, error) { return "", nil }, "x", "y")
		},
		want: "takes 1 arguments, given 2",
	}, {
		name: "variadic rule",
		fn:   func() { Derive[string]("k", fmt.Sprint, "x") },
		want: "variadic",
	}, {
		name: "return type disagrees with T",
		fn:   func() { Derive[int]("k", upper, "x") },
		want: "returns string, want int",
	}, {
		name: "does not return an error",
		fn:   func() { Derive[string]("k", strings.ToUpper, "x") },
		want: "want (string, error)",
	}, {
		name: "argument type disagrees with the rule",
		fn:   func() { Derive[string]("k", upper, 42) },
		want: "rule takes string, given int",
	}, {
		name: "dependency type disagrees with the rule",
		fn:   func() { Derive[string]("k", upper, Const("n", 1)) },
		want: "rule takes string, given int",
	}, {
		name: "unhashable parameter",
		fn:   func() { Derive[string]("k", func(float64) (string, error) { return "", nil }, 1.5) },
		want: "cannot hash float64",
	}, {
		name: "unexported field in a parameter",
		fn: func() {
			type p struct{ hidden string }
			Derive[string]("k", func(p) (string, error) { return "", nil }, p{"x"})
		},
		want: "unexported fields cannot be hashed",
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("did not panic")
				}
				if got := fmt.Sprint(r); !strings.Contains(got, tt.want) {
					t.Errorf("panic = %q, want it to contain %q", got, tt.want)
				}
			}()
			tt.fn()
		})
	}
}

// TestSharedKeyWithADifferentTypePanics checks the guard on the mistake a key
// derived from too little input produces: two artifacts that stand for
// different values agreeing on one.
//
// The nodes are built here rather than declared, because no declaration reaches
// this: a key covers the rule and the arguments, or the kind, the type and the
// value, and two artifacts that agree on all of that are one artifact. What is
// left is two rules the linker compiled to one body, which cannot be written
// down on purpose.
func TestSharedKeyWithADifferentTypePanics(t *testing.T) {
	c := unbounded()
	key := Key{'s', 'h', 'a', 'r', 'e', 'd'}
	str := Artifact[string]{n: &node{
		key:  key,
		kind: "str",
		typ:  reflect.TypeFor[string](),
		rule: func(*eval) (any, error) { return "a string", nil },
	}}
	num := Artifact[int]{n: &node{
		key:  key,
		kind: "num",
		typ:  reflect.TypeFor[int](),
		rule: func(*eval) (any, error) { return 42, nil },
	}}
	get(t, c, str)

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("did not panic")
		}
		if got := fmt.Sprint(r); !strings.Contains(got, "share a key") {
			t.Errorf("panic = %q, want it to mention a shared key", got)
		}
	}()
	num.Get(t.Context(), c)
}

// TestAValueThatMarshalsItselfIsAParameter checks the types whose fields the
// struct case cannot reach: a time is hashed by what it marshals to, so a
// struct holding one can be a parameter.
func TestAValueThatMarshalsItselfIsAParameter(t *testing.T) {
	type stamped struct {
		Name string
		At   time.Time
	}
	at := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

	if Const("t", at).Key() != Const("t", at).Key() {
		t.Error("one time produced two keys")
	}
	if Const("t", at).Key() == Const("t", at.Add(time.Second)).Key() {
		t.Error("two times share a key")
	}
	if Const("s", stamped{"a", at}).Key() == Const("s", stamped{"a", at.Add(time.Second)}).Key() {
		t.Error("changing a time in a struct did not change its key")
	}
	if Const("s", stamped{"a", at}).Key() == Const("s", stamped{"b", at}).Key() {
		t.Error("changing a name in a struct did not change its key")
	}
}

// TestKeysCoverTheType checks that two values whose bytes are the same but
// whose types differ are two artifacts, which is what keeps the guard above out
// of reach.
func TestKeysCoverTheType(t *testing.T) {
	type name string
	if Const("n", int32(1)).Key() == Const("n", int64(1)).Key() {
		t.Error("an int32 and an int64 of the same number share a key")
	}
	if Const("n", "x").Key() == Const("n", []byte("x")).Key() {
		t.Error("a string and the bytes of that string share a key")
	}
	if Const("n", name("x")).Key() == Const("n", "x").Key() {
		t.Error("a named string and a string of the same text share a key")
	}
}

// TestKeysDistinguishArgumentBoundaries checks that the key covers where one
// argument ends and the next begins, so that moving a character between two of
// them is a different key.
func TestKeysDistinguishArgumentBoundaries(t *testing.T) {
	two := func(a, b string) (string, error) { return a + b, nil }
	x := Derive[string]("two", two, "ab", "c")
	y := Derive[string]("two", two, "a", "bc")
	if x.Key() == y.Key() {
		t.Error(`("ab", "c") and ("a", "bc") hash alike`)
	}
}

// TestDifferentRulesDoNotShareAKey checks what separates two artifacts computed
// from the same input: the rule, not the name it was declared under. plan.go
// derives a document's summary requests and its body requests from one document
// artifact, and nothing but the rule tells the two apart.
func TestDifferentRulesDoNotShareAKey(t *testing.T) {
	c := unbounded()
	summary := func(s string) (string, error) { return "summary of " + s, nil }
	body := func(s string) (string, error) { return "body of " + s, nil }

	doc := Const("doc", "d")
	a := Derive[string]("requests", summary, doc)
	b := Derive[string]("requests", body, doc)

	if a.Key() == b.Key() {
		t.Fatal("two rules over the same input share a key")
	}
	if got := get(t, c, a); got != "summary of d" {
		t.Errorf("Get() = %q, want %q", got, "summary of d")
	}
	if got := get(t, c, b); got != "body of d" {
		t.Errorf("Get() = %q, want %q", got, "body of d")
	}
}

// TestKindNamesAnArtifactAndDecidesNothing checks the other half: the same rule
// over the same arguments is one artifact however it is named, so a kind that is
// wrong mislabels a count and cannot produce a wrong value.
func TestKindNamesAnArtifactAndDecidesNothing(t *testing.T) {
	c := unbounded()
	rule, runs := counted(strings.ToUpper)

	x := Derive[string]("one", rule, "a")
	y := Derive[string]("two", rule, "a")
	if x.Key() != y.Key() {
		t.Error("the same rule and arguments produced two keys")
	}
	get(t, c, x)
	get(t, c, y)
	if *runs != 1 {
		t.Errorf("rule ran %d times, want 1", *runs)
	}
}

// TestStructTypesDoNotCollide checks that a key covers the type of a struct
// parameter and not only its fields. Two kinds of request shaped alike -- and a
// closed set of them tends to be -- would otherwise share an artifact, and one
// would be served the other's value.
func TestStructTypesDoNotCollide(t *testing.T) {
	type highlight struct{ Text string }
	type parsediff struct{ Text string }

	c := unbounded()
	a := Derive[string]("fragment", func(highlight) (string, error) { return "highlighted", nil }, highlight{"x"})
	b := Derive[string]("fragment", func(parsediff) (string, error) { return "parsed", nil }, parsediff{"x"})

	if a.Key() == b.Key() {
		t.Fatal("two struct types with the same fields share a key")
	}
	if got := get(t, c, a); got != "highlighted" {
		t.Errorf("Get() = %q, want %q", got, "highlighted")
	}
	if got := get(t, c, b); got != "parsed" {
		t.Errorf("Get() = %q, want %q", got, "parsed")
	}
}

// TestConcurrentBuildAndServe reproduces what serve does: the old site keeps
// answering requests while the next build is declared and forced. A request
// that reaches a collection computes the artifacts under it as it goes, so the
// cache is written to from both sides at once.
func TestConcurrentBuildAndServe(t *testing.T) {
	elems := make([]string, 200)
	for i := range elems {
		elems[i] = strconv.Itoa(i)
	}

	c := unbounded()
	list := Collect[string]("uppers", plan("upper", upper, elems))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, err := list.Get(t.Context(), c); err != nil {
			t.Errorf("Get() = %v", err)
		}
	}()

	for i := range 200 {
		get(t, c, Derive[string]("upper", upper, strconv.Itoa(i)))
	}
	wg.Wait()
}
