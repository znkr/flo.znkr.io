// Package build declares values derived from one another, and computes them
// against a cache of what has been computed before.
//
// A value is declared with [Derive], which names the rule that computes it and
// the arguments it is computed from, and returns an [Artifact] standing for the
// result. Declaring touches nothing: no rule runs and no cache is consulted, so
// a build can be declared in full and then asked for one page of.
//
// [Artifact.Get] is where a [Cache] comes in. A key is derived from the rule
// and the arguments, so a value is valid for any build that produces the same
// key, and a declaration is worth rebuilding rather than keeping. The cache
// holds results and nothing else, which is what lets it be bounded: an artifact
// carries everything its rule needs, so a value it has dropped costs the work
// to compute again and nothing more.
//
// A value the build does not compute enters through [Const], which keys it by
// the value itself. No key is given from outside, so two artifacts share a key
// only when they stand for the same value.
//
// A rule is an ordinary function. It is given values, never artifacts, so it
// can depend on nothing that is not an argument. A rule whose first argument is
// a [context.Context] is given the context passed to [Artifact.Get]. That
// argument is not part of the key: it bounds how long the caller waits, it does
// not say what the value is. A rule that stops because its context was canceled
// produces no value, so nothing is held for it and the next caller computes it
// again.
package build

import (
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// Key identifies an artifact by its inputs.
type Key [32]byte

func (k Key) String() string { return hex.EncodeToString(k[:]) }

// Artifact is a value the build can produce: a key, the rule that computes it,
// and the artifacts it is computed from. Declaring one computes nothing and
// stores nothing.
type Artifact[T any] struct{ n *node }

// Get returns the artifact's value, computing it and whatever it depends on if
// c does not already hold it. Rules that take a context are given ctx, and Get
// returns ctx.Err() if ctx is canceled before the value is ready. Nothing a
// cancellation stopped is cached.
//
// It is safe for concurrent use, and concurrent calls for one artifact compute
// it once.
func (a Artifact[T]) Get(ctx context.Context, c *Cache) (T, error) {
	v, err := c.get(ctx, a.n)
	if err != nil {
		var zero T
		return zero, err
	}
	if v == nil {
		var zero T
		return zero, nil
	}
	return v.(T), nil
}

// Key returns the key the artifact is memoized under.
func (a Artifact[T]) Key() Key { return a.n.key }

// dep is the view of an [Artifact] that does not name its value type. It is
// unexported so that a dependency can only be an artifact.
type dep interface {
	depNode() *node
	depType() reflect.Type
}

func (a Artifact[T]) depNode() *node        { return a.n }
func (a Artifact[T]) depType() reflect.Type { return reflect.TypeFor[T]() }

var (
	binaryIface = reflect.TypeFor[encoding.BinaryMarshaler]()
	ctxIface    = reflect.TypeFor[context.Context]()
	depIface    = reflect.TypeFor[dep]()
	errIface    = reflect.TypeFor[error]()
)

// node is one artifact's declaration. It holds everything its rule needs, so a
// value the cache has dropped can always be computed again.
type node struct {
	key  Key
	kind string
	typ  reflect.Type
	rule func(*eval) (any, error)
}

// eval is one computation in progress: the context it runs under, the cache it
// reaches its inputs through, and the keys it has reached so far.
type eval struct {
	ctx   context.Context
	c     *Cache
	trace *[]Key
}

// get returns what n computes to, and records that this computation needed it.
func (e *eval) get(n *node) (any, error) {
	*e.trace = append(*e.trace, n.key)
	return e.c.get(e.ctx, n)
}

// result is one artifact's value, and what computing it took.
type result struct {
	kind string
	typ  reflect.Type

	// done reports whether val, err and trace are final. wait is closed when
	// the computation in flight finishes, and is nil when there is none. Both
	// are read and written under the cache's lock.
	done bool
	wait chan struct{}
	val  any
	err  error
	// trace are the keys computing this value reached, written once under the
	// cache's lock when the rule returns. Reaching a value reaches these too,
	// so that what a wanted value was made of is wanted.
	trace []Key
	// used is the cache's clock when this value was last reached.
	used uint64
}

// Stat counts the lookups of one kind.
type Stat struct{ Hits, Misses int }

// Stats counts lookups, in total and per kind.
type Stats struct {
	Hits   int
	Misses int
	Kinds  map[string]Stat
}

// Cache holds what artifacts have been computed to, keyed by their inputs.
//
// It holds results and nothing else: a declaration is rebuilt each build and
// carries what its rules need, so dropping a value costs the work to compute it
// again and nothing more. That is what lets the cache be bounded.
type Cache struct {
	mu    sync.Mutex
	vals  map[Key]*result
	clock uint64
	max   int
	stats map[string]Stat
}

// NewCache returns a cache holding at most max values. A max below one holds
// everything.
func NewCache(max int) *Cache {
	return &Cache{vals: make(map[Key]*result), max: max, stats: make(map[string]Stat)}
}

// Len returns the number of values held.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.vals)
}

// Stats returns the lookups since the last call to Stats.
func (c *Cache) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := Stats{Kinds: c.stats}
	for _, st := range c.stats {
		s.Hits += st.Hits
		s.Misses += st.Misses
	}
	c.stats = make(map[string]Stat)
	return s
}

func (c *Cache) get(ctx context.Context, n *node) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.mu.Lock()
	for {
		r, ok := c.vals[n.key]
		if ok {
			// A key stands for the value its inputs produce, so two artifacts
			// that share one and disagree about the type were keyed on too
			// little.
			if r.typ != n.typ {
				c.mu.Unlock()
				panic(fmt.Sprintf("build: %s (%s) and %s (%s) share a key", n.kind, n.typ, r.kind, r.typ))
			}
		} else {
			r = &result{kind: n.kind, typ: n.typ}
			c.vals[n.key] = r
		}
		c.reachLocked(r)

		if r.done {
			c.count(n.kind, true)
			c.mu.Unlock()
			return r.val, r.err
		}
		if r.wait != nil {
			wait := r.wait
			c.mu.Unlock()
			select {
			case <-wait:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			c.mu.Lock()
			// A canceled computation leaves nothing behind, so what was waited
			// for is looked up again rather than read off r.
			continue
		}

		// Nothing else is computing this one, so this call does.
		r.wait = make(chan struct{})
		c.mu.Unlock()
		return c.compute(ctx, n, r)
	}
}

// compute runs n's rule and records what it produced under r, the entry this
// call took charge of. It releases r however it leaves, a panic in the rule
// included: an entry whose wait channel is never closed blocks every later
// lookup of the key for as long as the cache lives.
func (c *Cache) compute(ctx context.Context, n *node, r *result) (v any, err error) {
	var trace []Key
	ran := false
	defer func() {
		c.mu.Lock()
		if !ran || dropped(ctx, err) {
			// A cancellation says nothing about the value, and neither does a
			// panic, so the entry goes and the next caller computes it again.
			if c.vals[n.key] == r {
				delete(c.vals, n.key)
			}
		} else {
			r.val, r.err, r.trace, r.done = v, err, trace, true
			c.evictLocked()
		}
		c.count(n.kind, false)
		close(r.wait)
		r.wait = nil
		c.mu.Unlock()
	}()
	v, err = n.rule(&eval{ctx: ctx, c: c, trace: &trace})
	ran = true
	return v, err
}

// dropped reports whether a rule that returned err was stopped by ctx rather
// than by the value it was computing. The context is consulted as well as the
// error because a rule can report a cancellation as an error that does not wrap
// one.
func dropped(ctx context.Context, err error) bool {
	return err != nil && (canceled(err) || ctx.Err() != nil)
}

// canceled reports whether err is a context cancellation or timeout.
func canceled(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// count records a lookup of this kind. The caller holds c.mu.
func (c *Cache) count(kind string, hit bool) {
	st := c.stats[kind]
	if hit {
		st.Hits++
	} else {
		st.Misses++
	}
	c.stats[kind] = st
}

// reachLocked stamps r, and everything computing r reached, with the current
// clock. The caller holds c.mu.
//
// The walk is what keeps a value nothing asks for directly: the artifacts under
// a [Collect] are reached by computing the collection, never by a build.
func (c *Cache) reachLocked(r *result) {
	c.clock++
	now := c.clock
	stack := []*result{r}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if x.used == now {
			continue // already reached in this walk
		}
		x.used = now
		for _, k := range x.trace {
			if y, ok := c.vals[k]; ok {
				stack = append(stack, y)
			}
		}
	}
}

// evictLocked drops the coldest tenth once the cache is over its bound. Only a
// result that is finished and has no waiters left can go: dropping one that a
// call is still waiting on makes that call compute it again. The caller holds
// c.mu.
//
// A scan rather than a list: going over the bound is rare, the map is small,
// and neither is worth the links.
func (c *Cache) evictLocked() {
	if c.max < 1 || len(c.vals) <= c.max {
		return
	}
	keep := max(c.max-c.max/10, 1)
	stamps := make([]uint64, 0, len(c.vals))
	for _, r := range c.vals {
		stamps = append(stamps, r.used)
	}
	slices.Sort(stamps)
	cut := stamps[len(stamps)-keep]
	for k, r := range c.vals {
		if evictable(r) && r.used < cut {
			delete(c.vals, k)
		}
	}
}

// evictable reports whether r can be dropped from the cache.
func evictable(r *result) bool {
	return r.done && r.wait == nil
}

// Const returns an artifact holding v under a key derived from kind and v. It
// is how a value the build did not compute -- a file read from disk, say --
// enters the graph. The value is held by the declaration, so the cache dropping
// it loses nothing.
//
// kind names the artifact in the counts [Cache.Stats] reports, as it does in
// [Derive].
//
// Const panics if v is not one of the parameter types [Derive] documents.
func Const[T any](kind string, v T) Artifact[T] {
	typ := reflect.TypeFor[T]()
	h := sha256.New()
	writePart(h, []byte(kind))
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		panic(fmt.Sprintf("build: %s: cannot hash nil", kind))
	}
	hashParam(h, rv, kind)
	var key Key
	h.Sum(key[:0])

	return Artifact[T]{n: &node{
		key:  key,
		kind: kind,
		typ:  typ,
		rule: func(*eval) (any, error) { return v, nil },
	}}
}

// Derive declares an artifact computed by fn from args.
//
// Each argument is either a dependency -- an [Artifact] or a slice of them,
// passed to fn as the value it holds -- or a parameter, passed to fn as itself.
// A parameter may be a string, a bool, a sized integer, a byte slice, a [Key],
// an [encoding.BinaryMarshaler] such as a [time.Time], or a slice, array or
// struct of those. A struct's fields must all be exported, unless it marshals
// itself. Both kinds are part of the key, so fn can depend on nothing that is
// not an argument.
//
// The key is derived from fn and args. kind names the artifact in the counts
// [Cache.Stats] reports and in the panics below, and decides nothing: two
// artifacts computed the same way from the same arguments are one artifact
// whatever they are called.
//
// Derive panics if an argument is neither a dependency nor a parameter, or if
// fn does not take the arguments given and return (T, error). It computes
// nothing and touches no cache: see [Artifact.Get].
func Derive[T any](kind string, fn any, args ...any) Artifact[T] {
	fv, ft, ctxArg := checkRule(kind, fn, reflect.TypeFor[T](), len(args))

	// The rule's own arguments start after the context, if it takes one.
	off := 0
	if ctxArg {
		off = 1
	}

	h := sha256.New()
	hashRule(h, fv)
	for i, a := range args {
		hashArg(h, a, ft.In(i+off), fmt.Sprintf("%s: argument %d", kind, i))
	}
	var key Key
	h.Sum(key[:0])

	return Artifact[T]{n: &node{
		key:  key,
		kind: kind,
		typ:  reflect.TypeFor[T](),
		rule: func(ev *eval) (any, error) { return callRule(ev, fv, ft, ctxArg, args) },
	}}
}

// Collect declares an artifact holding the values of the artifacts src
// produces, keyed by the artifact each was computed under.
//
// It is how a step whose units of work are only known once an earlier one has
// run -- the fragments of a document, say -- stays one artifact per unit rather
// than one for the whole batch. A caller that holds one of those artifacts
// looks its value up under its key, so nothing has to line up by position.
//
// kind names the collection in the counts [Cache.Stats] reports. The artifacts
// it holds keep the kinds they were declared with.
func Collect[T any](kind string, src Artifact[[]Artifact[T]]) Artifact[map[Key]T] {
	h := sha256.New()
	writePart(h, []byte("build.Collect"))
	writePart(h, src.n.key[:])
	var key Key
	h.Sum(key[:0])

	return Artifact[map[Key]T]{n: &node{
		key:  key,
		kind: kind,
		typ:  reflect.TypeFor[map[Key]T](),
		rule: func(ev *eval) (any, error) {
			v, err := ev.get(src.n)
			if err != nil {
				return nil, err
			}
			elems, _ := v.([]Artifact[T])
			out := make(map[Key]T, len(elems))
			for _, a := range elems {
				// Through ev, so that the artifacts are in the collection's
				// trace: nothing else ever asks for them.
				v, err := ev.get(a.n)
				if err != nil {
					return nil, err
				}
				if v != nil {
					out[a.n.key] = v.(T)
				}
			}
			return out, nil
		},
	}}
}

// checkRule reports fn's reflected form and whether its first argument is a
// [context.Context], panicking unless it takes n arguments besides that one and
// returns (out, error).
func checkRule(kind string, fn any, out reflect.Type, n int) (reflect.Value, reflect.Type, bool) {
	fv := reflect.ValueOf(fn)
	if !fv.IsValid() || fv.Kind() != reflect.Func {
		panic(fmt.Sprintf("build: %s: rule is %T, want a function", kind, fn))
	}
	ft := fv.Type()
	if ft.IsVariadic() {
		panic(fmt.Sprintf("build: %s: rule is variadic", kind))
	}
	ctxArg := ft.NumIn() > 0 && ft.In(0) == ctxIface
	want := n
	if ctxArg {
		want++
	}
	if ft.NumIn() != want {
		panic(fmt.Sprintf("build: %s: rule takes %d arguments, given %d", kind, ft.NumIn()-(want-n), n))
	}
	if ft.NumOut() != 2 || ft.Out(1) != errIface {
		panic(fmt.Sprintf("build: %s: rule returns %s, want (%s, error)", kind, outs(ft), out))
	}
	if ft.Out(0) != out {
		panic(fmt.Sprintf("build: %s: rule returns %s, want %s", kind, ft.Out(0), out))
	}
	return fv, ft, ctxArg
}

// hashRule writes the identity of the rule fv holds, so that two rules over the
// same arguments are two artifacts and one rule twice is one artifact, without
// anyone having to say so.
//
// The identity is the code pointer. Two functions compiled to one body share
// it, which is what should happen: bodies that are identical compute the same
// thing from the same arguments. A closure shares it with every other closure
// over that body, so what one captures must not decide what it returns -- which
// is what [Derive] requires of a rule anyway.
func hashRule(h hash.Hash, fv reflect.Value) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(fv.Pointer()))
	writePart(h, b[:])
}

func outs(ft reflect.Type) string {
	var s strings.Builder
	for i := 0; i < ft.NumOut(); i++ {
		if i > 0 {
			s.WriteString(", ")
		}
		s.WriteString(ft.Out(i).String())
	}
	return "(" + s.String() + ")"
}

// hashArg writes the part of the key that arg contributes, and checks that it
// can be passed as want.
func hashArg(h hash.Hash, arg any, want reflect.Type, path string) {
	if d, ok := arg.(dep); ok {
		assignable(d.depType(), want, path)
		k := d.depNode().key
		writePart(h, k[:])
		return
	}

	v := reflect.ValueOf(arg)
	if v.IsValid() && v.Kind() == reflect.Slice && v.Type().Elem().Implements(depIface) {
		if v.Len() > 0 {
			assignable(v.Index(0).Interface().(dep).depType(), want.Elem(), path)
		}
		binary.Write(h, binary.LittleEndian, uint64(v.Len()))
		for i := range v.Len() {
			k := v.Index(i).Interface().(dep).depNode().key
			writePart(h, k[:])
		}
		return
	}

	if !v.IsValid() {
		panic(fmt.Sprintf("build: %s: cannot hash nil", path))
	}
	assignable(v.Type(), want, path)
	hashParam(h, v, path)
}

func assignable(got, want reflect.Type, path string) {
	if want == nil || got.AssignableTo(want) {
		return
	}
	panic(fmt.Sprintf("build: %s: rule takes %s, given %s", path, want, got))
}

// hashParam writes v, which must be one of the parameter types [Derive]
// documents.
func hashParam(h hash.Hash, v reflect.Value, path string) {
	// The type goes in ahead of the value, because the value alone does not say
	// what it is: an int32 and an int64 of the same number both write eight
	// bytes, and two structs shaped alike write the same fields.
	writePart(h, []byte(v.Type().String()))

	// A type that says how to serialize itself is hashed by what it marshals
	// to. It comes before the switch because such a type is usually a struct
	// whose fields are unexported, as time.Time is.
	if v.Type().Implements(binaryIface) {
		b, err := v.Interface().(encoding.BinaryMarshaler).MarshalBinary()
		if err != nil {
			panic(fmt.Sprintf("build: %s: MarshalBinary: %v", path, err))
		}
		writePart(h, b)
		return
	}

	switch v.Kind() {
	case reflect.String:
		writePart(h, []byte(v.String()))

	case reflect.Bool:
		var b [1]byte
		if v.Bool() {
			b[0] = 1
		}
		writePart(h, b[:])

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], uint64(v.Int()))
		writePart(h, b[:])

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		var b [8]byte
		binary.LittleEndian.PutUint64(b[:], v.Uint())
		writePart(h, b[:])

	case reflect.Slice, reflect.Array:
		binary.Write(h, binary.LittleEndian, uint64(v.Len()))
		if v.Type().Elem().Kind() == reflect.Uint8 {
			if v.Kind() == reflect.Slice {
				writePart(h, v.Bytes())
			} else {
				// reflect.Value.Bytes requires an addressable array, and nothing
				// reached from an argument is addressable.
				b := make([]byte, v.Len())
				reflect.Copy(reflect.ValueOf(b), v)
				writePart(h, b)
			}
			return
		}
		for i := range v.Len() {
			hashParam(h, v.Index(i), fmt.Sprintf("%s[%d]", path, i))
		}

	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				panic(fmt.Sprintf("build: %s.%s: unexported fields cannot be hashed", path, f.Name))
			}
			writePart(h, []byte(f.Name))
			hashParam(h, v.Field(i), path+"."+f.Name)
		}

	default:
		panic(fmt.Sprintf("build: %s: cannot hash %s", path, v.Kind()))
	}
}

// writePart writes b with its length in front, so that no two different
// sequences of parts hash alike.
func writePart(h hash.Hash, b []byte) {
	binary.Write(h, binary.LittleEndian, uint64(len(b)))
	h.Write(b)
}

// callRule resolves args to the values fn takes and runs it on them, passing
// ev's context first if fn takes one.
func callRule(ev *eval, fv reflect.Value, ft reflect.Type, ctxArg bool, args []any) (any, error) {
	in := make([]reflect.Value, 0, ft.NumIn())
	if ctxArg {
		in = append(in, reflect.ValueOf(ev.ctx))
	}
	for _, a := range args {
		v, err := argValue(ev, a, ft.In(len(in)))
		if err != nil {
			return nil, err
		}
		in = append(in, v)
	}
	out := fv.Call(in)
	if err, _ := out[1].Interface().(error); err != nil {
		return nil, err
	}
	return out[0].Interface(), nil
}

// argValue returns arg as the value the rule takes, computing the artifacts it
// names.
func argValue(ev *eval, arg any, want reflect.Type) (reflect.Value, error) {
	set := func(v any) reflect.Value {
		if v == nil {
			return reflect.Zero(want)
		}
		return reflect.ValueOf(v)
	}

	if d, ok := arg.(dep); ok {
		v, err := ev.get(d.depNode())
		if err != nil {
			return reflect.Value{}, err
		}
		return set(v), nil
	}

	rv := reflect.ValueOf(arg)
	if rv.IsValid() && rv.Kind() == reflect.Slice && rv.Type().Elem().Implements(depIface) {
		out := reflect.MakeSlice(want, rv.Len(), rv.Len())
		for i := range rv.Len() {
			v, err := ev.get(rv.Index(i).Interface().(dep).depNode())
			if err != nil {
				return reflect.Value{}, err
			}
			if v != nil {
				out.Index(i).Set(reflect.ValueOf(v))
			}
		}
		return out, nil
	}
	return set(arg), nil
}
