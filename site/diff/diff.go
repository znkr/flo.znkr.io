//go:build ignore

package diff

// Op describes an edit operation.
type Op int

const (
	Match  Op = iota // Two slice elements match
	Delete           // A deletion from an element on the left slice
	Insert           // An insertion of an element from the right side
)

// Edit describes a single edit of a diff.
// - For Match, both X and Y contain the matching element.
// - For Delete, X contains the deleted element and Y is unset (zero value).
// - For Insert, Y contains the inserted element and X is unset (zero value).
type Edit[T any] struct {
	Op   Op
	X, Y T
}

// Hunk describes a sequence of consecutive edits.
type Hunk[T any] struct {
	PosX, EndX int       // Start and end position in x.
	PosY, EndY int       // Start and end position in y.
	Edits      []Edit[T] // Edits to transform x[PosX:EndX] to y[PosY:EndY]
}

// Edits compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
//
// Edits returns one edit for every element in the input slices. If x and y are identical, the
// output will consist of a match edit for every input element.
func Edits[T comparable](x, y []T, opts ...Option) []Edit[T]

// Hunks compares the contents of x and y and returns the changes necessary to convert from one to
// the other.
//
// The output is a sequence of hunks. A hunk represents a contiguous block of changes (insertions
// and deletions) along with some surrounding context.
func Hunks[T comparable](x, y []T, opts ...Option) []Hunk[T]

// EditsFunc compares the contents of x and y using the provided equality comparison and returns the
// changes necessary to convert from one to the other.
func EditsFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Edit[T]

// HunksFunc compares the contents of x and y using the provided equality comparison and returns the
// changes necessary to convert from one to the other.
func HunksFunc[T any](x, y []T, eq func(a, b T) bool, opts ...Option) []Hunk[T]
