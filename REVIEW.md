# Repository review

Reviewed 2026-10-27. Scope: all Go source, tests, scan profile, README, and build tasks. No production code changed. Findings distinguish reproduced behavior from static code observations.

## Assessment

The small codebase has a useful separation between argument parsing, scanning, statistics, and presentation. The primary problem is trust: several inputs are silently reinterpreted or ignored, and successful output does not say whether the scan was complete or what was selected. Improve correctness and explainability before adding more statistics.

## Prioritized findings

### 1. High: missing explicit configuration silently selects another profile

Location: `config.go:34–67`.

Reproduced: `codesiz --config definitely-missing-profile.json <fixture>` successfully analyzes files using the local default profile. A typo changes the analysis instead of producing an error. `fileExists` also treats all stat errors as absence. A working-directory `languages.json` overrides the embedded profile, so identical invocations from different directories can select different rules.

Recommendation: distinguish an omitted config flag from an explicit filename. Always fail on an unreadable explicit filename; use the embedded profile by default, or document and report a deliberate local-profile discovery policy. Include profile source in results. Validate unknown JSON fields, empty/null profiles, malformed patterns, and trailing JSON consistently; currently `{}` and `null` effectively allow all files through the include stage.

### 2. High: output destination is silently ignored for two formats

Location: `output.go:12–54`.

Reproduced: `codesiz --output report.txt <fixture>` prints to stdout and creates no file. Histogram has the same code path. Help advertises `--output` without a JSON restriction.

Recommendation: choose the destination once for every renderer. Propagate text/histogram write errors, which are currently discarded. Check file-close errors. For file output, render to a temporary file in the destination directory, close successfully, then replace the destination using appropriate platform behavior. Reject output paths that overwrite scanned source or configuration. JSON currently uses `os.Create`, so specifying a source file as output can truncate it after scanning.

### 3. High: ignore and glob behavior can select the wrong files

Location: `scan.go:136–174, 227–285`.

Reproduced: a root `.gitignore` containing `/a.go` does not exclude root `a.go`. Root anchoring is left in the pattern while relative candidate paths have no leading slash.

Static observations:

- Recursive glob handling is a collection of special cases. `src/**/*.go` does not provide general recursive matching, and `**/test/*.go` fails to match `a/test/x.go`.
- `**/vendor/**` does not match the nested directory `a/vendor` itself, although descendant files match. This matters if directory pruning is added.
- Malformed `path.Match` patterns discard their errors, making user mistakes look like an empty scan.
- Paths and patterns are unconditionally lowercased, changing matching semantics on case-sensitive systems and for Git ignore rules.
- Slashless Git patterns are checked against every path component even for file-only matches; rules, re-inclusion, escapes, and directory wildcard handling need broader compatibility tests.
- Only the scanned root's `.gitignore` is loaded. This limitation is documented, but nested rules and parent repository context will still surprise users expecting Git behavior.

Recommendation: use a maintained, well-tested implementation for the chosen glob dialect and Git ignore semantics, or explicitly specify a smaller dialect. Add fixtures for root anchoring, recursive paths, directory-only rules, negation, escaped markers, malformed patterns, and platform case behavior. Offer `--no-gitignore` and an explanation mode showing which rule selected or rejected a path.

### 4. High: partial scans look like successful complete scans

Location: `scan.go:48–80`, `AnalysisResult`.

Static observation: a file read error is logged and scanning continues, but the result contains no warning count, affected paths, or completeness field and the process can exit successfully. Conversely, a directory traversal error aborts the whole scan. Consumers of JSON cannot establish completeness from JSON alone.

Recommendation: define a consistent policy: strict by default, or explicitly partial with structured warnings and a documented exit status. Report discovered, selected, successfully read, skipped, failed, and deliberately dropped counts separately. Include the failing operation, path, and suggested recovery in diagnostics.

### 5. Medium: documented build command fails

Location: `README.md:89,95`.

Reproduced: `go build -o codesiz.exe main.go` fails with undefined `loadConfig`, `analyze`, and `writeResult`. The program now spans several source files. The Mage build already uses the correct package target.

Recommendation: document `go build -o codesiz.exe .` on Windows and `go build -o codesiz .` elsewhere. Add install, executable discovery, and first-run examples. Verify README commands in CI.

### 6. Medium: ordinary invocation mistakes produce misleading errors

Location: `main.go:66–158`, `scan.go:41–64`.

Reproduced: `codesiz <folder> --list` reports “expected one folder argument, got 2” because standard flag parsing stops at the first positional argument. Passing an existing regular file returns “No files found” with success because the root entry is skipped without directory validation.

Recommendation: accept flags before or after the path, or diagnose misplaced options directly. Default an omitted path to `.`. Validate root existence and type before reading `.gitignore`. Send argument errors and diagnostics to stderr, with concise usage examples and documented exit codes. Currently parser diagnostics use stdout and fatal errors have log timestamps. Show allowed values when rejecting formats or sorting modes.

### 7. Medium: equal-sized files produce misleading clusters

Location: `stats.go:54,103–122,185–237`.

Reproduced: three one-line files produce “Large: 3 files” and empty “Small” and “Medium” groups. Duplicate centroid seeds leave empty clusters; their artificial zero averages participate in labeling.

Recommendation: use at most the number of distinct sizes, remove empty clusters, order nonempty groups by size, and suppress clustering when the data cannot support meaningful groups. State that labels are relative to this scan and do not measure code quality. Ten iterations also impose a convergence limit without reporting convergence; either implement a tested stopping policy or use a simpler, explained grouping method.

### 8. Medium: exclusions do not prune expensive directory traversal

Location: `scan.go:60–71,107–133`.

Static observation: profile and command-line excludes are checked only for files. Excluded `node_modules`, build trees, and `.git` directories are still walked unless `.gitignore` prunes them. This increases scan time and can expose traversal errors inside trees users deliberately excluded.

Recommendation: compile patterns once and prune directories when rules safely exclude the entire subtree. Avoid constructing combined exclude slices for every file. Benchmark a realistic dependency-heavy tree before adding concurrency.

### 9. Medium: results lack enough context for reliable interpretation

Location: `scan.go:25–34`, `stats.go:60–100`, `output.go:56–80`.

The measurement counts physical lines, including comments and blanks, with a final unterminated line counted once. This is useful but should be explicit. “Standard deviation (high/low)” is actually the root mean square distance from the overall mean, computed separately above and below it; it is not standard deviation within each subgroup. Rounding all text statistics to integers hides half-line medians.

JSON lacks a schema version, root, effective filters, profile source, measurement definition, and completeness status. Empty results serialize zero-valued smallest/largest file objects even though no such file exists. Paths reflect the supplied root spelling, making reports from different machines or invocations harder to compare.

Recommendation: use clear names and units; prioritize median, p90/p95, totals, and largest files. Keep advanced dispersion optional and document its exact formula. Use root-relative report paths and explicit null/absent unavailable measurements. Record effective scan settings and dropped-file totals alongside post-filter totals.

### 10. Low to medium: output presentation does not scale well

Location: `scan.go:202–211`, `output.go:84–110`.

Line sorting is ascending, which hides the largest files at the bottom. `--sort` alone has no visible file-list effect. “Histogram” is a per-file bar chart rather than a binned distribution. One enormous file compresses all other bars; long absolute paths and Unicode display widths complicate terminal alignment. There is no ASCII fallback or limit on rows.

Recommendation: support explicit ascending/descending order, a default largest-first top 10, and `--top N`. Separate per-file bars from a true line-count distribution. Offer stable bins or a labeled logarithmic scale, root-relative paths, terminal-width-aware truncation, and ASCII rendering. Preserve full paths in JSON.

## Recommended user experience

First run: `codesiz` scans the current directory using a named built-in profile. The report begins with the root, profile, ignore policy, physical-line definition, and completeness. An empty result explains why candidates were rejected and suggests an appropriate next command.

Suggested default report (illustrative numbers, not this repository's measurements):

```text
Source size: ./example
Profile: built-in source | Git ignores: enabled | Complete: yes
120 files analyzed; 15,420 physical lines (includes blanks and comments)
Median: 74.5 lines | 90th percentile: 280 lines
Skipped: 850 by filters, 32 by Git ignore; read failures: 0

Largest files
  src/parser.go       1,240 lines
  src/service.go        810 lines

File length helps locate review candidates; it does not measure complexity.
```

Add `--explain <path>` for selection decisions and `--dry-run` for reviewing selection without reading file contents. Provide examples for common tasks: finding the biggest files, excluding generated code, scanning a monorepo subtree, exporting JSON, and comparing runs. Explain that `--all` currently bypasses includes only: profile excludes and Git ignores still apply. Consider a clearer name or separate controls.

`--drop-largest` should display the original totals, removed totals, and remaining totals, and warn when it removes every selected file. Do not encourage removing outliers as a default; large files may be the most useful findings.

## Implementation sequence and verification

1. Restore trustworthy operation: build documentation, explicit config errors, root validation, shared output destination, safe file writes, and structured partial-scan status.
2. Restore trustworthy selection: define and test glob/ignore semantics, report effective settings, add selection explanations, then optimize directory pruning.
3. Improve interpretation: physical-line definition, relative paths, top files and percentiles, truthful empty results, and removal or repair of misleading cluster groups.
4. Improve distribution and maintenance: CI on Windows and Linux, a documented minimum Go version, release binaries, version reporting, and executable-level smoke tests.

Regression checks should focus on observable contracts: custom-config failure, every format/destination combination, misplaced flags, file versus directory roots, empty/fully-dropped scans, ignore fixtures, uniform/two-size distributions, failing writers, partial scans, and output-file collisions. Use statistical invariants (counts sum to analyzed files, ranges contain members, no empty groups) rather than snapshots alone. Add bounded-memory line reading for very large newline-free files if large/binary scans are supported; `ReadString` currently allocates proportional to line length. Define a binary-file policy for `--all`.

## Verification performed

- `go test ./... -cover`: passed; 59.8% statement coverage.
- `go vet ./...`: passed.
- `go build -o <temporary executable> .`: passed.
- README single-file build: failed as described above.
- Temporary CLI fixtures reproduced missing-config fallback, ignored text output destination, flags-after-path error, regular-file empty success, ignored root-anchored ignore rule, and misleading equal-size clusters.

Permission failures, disk-full/close failures, Linux case behavior, and large-tree performance were assessed from code and were not reproduced in this Windows review. Existing tests cover core happy paths but do not cover these failure contracts. The histogram test also declares two total files while supplying three entries, weakening the consistency of that fixture.
