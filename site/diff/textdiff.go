//go:build ignore

package textdiff

import "znkr.io/diff"

// Unified compares the lines in x and y and returns the changes necessary to convert from one to
// the other in unified format.
func Unified[T string | []byte](x, y T, opts ...diff.Option) T

// Edit describes a single edit of a line-by-line diff.
type Edit[T string | []byte] struct {
	Op   diff.Op // Edit operation
	Line T       // Line, including newline character (if any)
}

// Hunk describes a sequence of consecutive edits.
type Hunk[T string | []byte] struct {
	PosX, EndX int       // Start and end line in x (zero-based).
	PosY, EndY int       // Start and end line in y (zero-based).
	Edits      []Edit[T] // Edits to transform x lines PosX..EndX to y lines PosY..EndY
}

// Edits compares the lines in x and y and returns the changes necessary to convert from one to the
// other.
func Edits[T string | []byte](x, y T, opts ...diff.Option) []Edit[T]

// Hunks compares the lines in x and y and returns the changes necessary to convert from one to the
// other.
func Hunks[T string | []byte](x, y T, opts ...diff.Option) []Hunk[T]
