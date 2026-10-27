# codesiz

codesiz is a command-line utility written in Go that analyzes source code file lengths.

![Codesiz](./codesiz.png)

![Clustering](./clustering.png)

## Features

- Recursively scans a directory for developer-interesting source files.
- Uses built-in source patterns, with optional JSON profiles and command-line overrides.
- Honors the root `.gitignore` by default.
- Computes total files, total lines, average, median, smallest/largest files, asymmetric standard deviations, and optional k-means file size clusters.
- Writes text, JSON, or histogram output.
- Supports ad hoc include/exclude patterns from the command line.

## Usage

```sh
codesiz [options] [path]
```

Common examples:

```sh
codesiz .
codesiz --list --sort lines .
codesiz --format histogram --sort lines .
codesiz --format json --output report.json .
codesiz --include "**/*.go,**/*.md" --exclude "vendor/**" .
codesiz --drop-largest 2 .
codesiz --all --exclude ".git/**,node_modules/**" .
```

Options:

```text
--all                 scan all regular files instead of profile includes
--config <file>       optional scan profile JSON file (default: built-in)
--discover            show file types and selection counts
--init                generate an optional profile (never overwrite)
--drop-largest <n>    exclude the largest n files from analysis
--exclude <pattern>   exclude glob pattern; repeat or use commas
--format <format>     output format: text, json, histogram (default "text")
--histogram           alias for --format histogram
--include <pattern>   include glob pattern; repeat or use commas
--list                include per-file details
--no-clusters         hide k-means file size clusters
--output <file>       write output to a file instead of stdout
--sort <mode>         sort file details: none, path, lines (default "none")
-h, --help            print help
-?                    print help
```

## Scan Profile

No config file is required. The defaults are embedded from `languages.json` when the program is built. Files named `languages.json` in the working directory are never loaded automatically. Use `--config <file>` to explicitly select a profile; missing or invalid profiles produce an error.

The path defaults to the current directory. Keep options before the path.

```sh
codesiz
codesiz --discover .
codesiz --init .
codesiz --config codesiz.json .
codesiz --include "**/*.sql" --discover .
codesiz --include "**/*.sql" --init --output sql-profile.json .
```

`--discover` inventories regular files by extension without reading their contents. It reports selected, profile/CLI-filtered, and root-Git-ignored counts. Unknown extensions are shown but never automatically selected. Discovery walks excluded directories too so it can report their file types; it can take time on large dependency trees. Symlinks are not followed.

Discovery explains filtered counts with the matching exclusion rule or “no include pattern matched.” For additional types it reports, for example, “Found 24 .sql file(s) excluded by the default profile” and suggests a preview command. Suggestions preserve existing includes and exclusions; they never change the analysis automatically. Files excluded by explicit rules or Git ignores are not suggested as additions.

`--init` shows discovery and writes an optional profile to `<path>/codesiz.json` (or `--output`). Existing files are never overwritten. With built-in defaults, the generated includes cover selected file types actually observed; future types require updating the profile. Built-in exclusions are retained. Explicit `--config` and `--include` selection patterns are preserved so custom path restrictions are not broadened. No selected types produces an actionable error instead of an allow-all profile. Review the generated file, then opt in using `--config`; it is never loaded automatically.

Discovery/init accept include/exclude overrides but cannot be combined with `--all`, per-file listings, dropping largest files, or non-text formats. `--output` on discovery alone is rejected; with `--init` it names the generated profile.

Custom profiles require a nonempty `include` array and reject unknown fields, null profiles, and trailing JSON. They describe useful files and exclusions rather than a language registry.

```json
{
  "include": [
    "**/*.go",
    "**/*.py",
    "**/*.ts"
  ],
  "exclude": [
    ".git/**",
    "node_modules/**",
    "**/*.generated.*",
    "**/*.min.js"
  ]
}
```

Pattern notes:

- `**/*.go` matches files by extension anywhere under the scanned path.
- `vendor/**` matches a directory at the scan root.
- `**/vendor/**` matches nested vendor directories.
- `*.md` matches a file name in any directory.
- Exclude patterns win over include patterns.
- Command-line `--include` patterns replace the profile includes for that run.
- Command-line `--exclude` patterns are added to the profile excludes.

codesiz also reads `.gitignore` from the scanned root and skips ignored files and directories before applying the scan profile.

## Building

```sh
go build -o codesiz.exe .
```

On Linux/macOS:

```sh
go build -o codesiz .
```

## Testing

```sh
go test ./...
```
