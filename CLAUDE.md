# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

The source and the static site generator for flo.znkr.io. Articles are written
in markst (a Typst-like markup language, `znkr.io/markst`), rendered to HTML by
the Go program in `generator`, and deployed to GitHub Pages as a tar archive.

## Commands

```
go run ./generator serve          # serve at localhost:8080, rebuilding on file changes
go run ./generator pack out.tar   # pack the whole site into a tar archive
go test ./...                     # all tests
go test ./generator -run TestGenerator          # golden end-to-end test
go test ./generator -update                     # rewrite testdata/golden after an intentional change
go test ./generator -bench . -run '^$'          # cache benchmarks
go vet ./...
```

`gofmt -l .` fails on `generator/markst/markst.go`: it holds a method with type
parameters, which Go 1.27 builds and vets but gofmt cannot parse. Do not
reformat around it, and do not trust a gofmt exit code as a lint result.

## Layout

- `site/` — the articles and assets, one directory per article with an
  `index.mst` and whatever it includes. `site/_assets` holds the CSS, JS and
  icons.
- `lib/` — markst libraries every document gets without an import: `article`,
  `page`, admonitions, math.
- `templates/` — `html/template` files. `article.html` and `page.html` are the
  two page templates, named by a document's `type` metadata; `fragments/` holds
  the partials, including the ones the snippet and diff includes are drawn with.
- `generator/` — the generator.

## Architecture

The generator is a build graph, not a render loop. Read
`generator/build/build.go`'s package comment first; the rest follows from it.

`build.Derive` declares a value: the rule that computes it and the artifacts it
is computed from. Declaring computes nothing. A key is derived from the rule and
its arguments, so a value is valid for any build producing the same key, and
`Cache` can be bounded — an artifact carries everything its rule needs, so a
dropped value costs only the recompute. A rule is a plain function given values,
never artifacts. A rule whose first argument is a `context.Context` gets the one
passed to `Artifact.Get`; that argument is not part of the key.

The pipeline:

1. `source.Scan` reads `templates`, `lib` and `site` into an immutable hashed
   `Tree`. It is the only filesystem boundary — everything downstream reads
   through the tree, so any input a step can reach is one that has been hashed.
   An `.ignore` file excludes its globs from becoming documents; the files are
   still read, because they are snippet and diff sources.
2. `plan` (in `generator/plan.go`) declares the whole site as artifacts and
   returns a `*site.Site` plus one artifact holding every diagnostic. This is
   where a change to what the site is made of goes: paths, metadata, which
   documents exist, what the index and the feed are built from.
3. `Doc.Page.Get` or `Doc.Content.Get` computes a document on demand, against
   the cache.

Granularity is deliberate. A document's summary, body, metadata, page and each
highlighted fragment are separate artifacts, so an edit recomputes only what it
changed. Keep it that way when adding steps.

Diagnostics are returned, never printed where they arise. A compile carries its
warnings in the value, so `collectDiags` can report them again for a document
this build did not recompile.

`serve` holds one cache for the life of the process and reloads the plan on file
changes; the cache survives the reload, which is what makes the rebuild cheap.
`pack` builds every document once, minifies CSS, JS, SVG and XML, and writes the
tar the deploy workflow uploads.

## Documents

A `.mst` file at `site/foo/index.mst` is served at `/foo`; any other name is
served at its path without the extension. A document's own directory is the
filesystem it can include from, so include paths are relative to it. A document
at the site root can include nothing.

The first call in an article is `#article(title:, published:, updated:,
summary:)`, from `lib/lib.mst`; it emits the `<doc-meta>` metadata the generator
reads. A page uses `#show: page.with(...)` instead. Metadata `type` selects the
template and must be one of `site.DocTypes`.

`#include-snippet` and `#include-diff` (`generator/markst/builtins`) read their file and resolve
the lexer and line range at compile time, so a missing file or a bad range is a
compile error pointing at the argument. The highlighting itself is deferred to
an artifact per fragment.

Files whose extension `mime.TypeByExtension` does not know (`.go`, `.diff`,
`.mod`) are served as plain text. Do not add extension special-cases to
`mimeType`; the fallback is the answer.

## Tests

`TestGenerator` loads the fixture site in `generator/testdata/root`, packs it,
and compares against `generator/testdata/golden`. The fixture brings its own
templates and library so the golden files stay small. Regenerate with `-update`
and read the diff before committing.

`TestPackSite` packs the real site in this repository, checking only that every
document loads, renders and lands in the archive.
