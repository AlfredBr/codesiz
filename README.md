# codesiz

codesiz is a command-line utility written in Go that analyzes source code file lengths.

![Codesiz](./codesiz.png)

## Features
- Recursively scans directories for files.
- Configurable file extensions via an external JSON configuration.
- Computes statistics such as total files, total lines, average lines, median lines, largest/smallest file size, and standard deviations.
- Output options:
  - `-?` help / usage information.
  - `-l` for detailed listing.
  - `-s` for sorted output (smallest to largest).
  - `-h` for a graphical histogram representation.
  - `-a` to analyze all files (not just those matching configured extensions).
  - `-j` for JSON output of the analysis results.
- Language filtering options:
  - `-i` to include only a specific language type (ignores languages.json).
  - `-e` to exclude a specific language file type as defined in languages.json.
- Additional options:
  - `-k` to exclude the largest *n* files from the analysis. For example, `-k 1` excludes the largest file, and `-k 2` excludes the two largest files. (The excluded files are displayed separately.)

## Building

1. Ensure you have Go installed.
2. Clone the repository.
3. Build for Windows:
```
go build -o codesiz.exe main.go
```
4. Build for Linux:
```
go build -o codesiz main.go
```

