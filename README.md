# fix.md

A command line tool to process, reformat and fix grammatical errors in markdown files.

Great for usage with Obsidian

## Features

- Formats and corrects spelling in Markdown files.
- Processes individual files or entire directories.
- Creates automatic backups with preserved directory structure.
- Uses Google's Gemini AI for text processing.

## Installation

1.  Clone this repository.
2.  Build the application:

    ```bash
    go build -o fixmd
    ```
3.  Make it executable:

    ```bash
    ```

    ```bash
    ```
**You need to place your own Gemini API key into the .env file for the program to work.**


## Usage

Process a single file:

```bash
./fixmd your_file.md
```

Process all Markdown files in a directory (non-recursive):

```bash
./fixmd your_directory/
```

Process all Markdown files in a directory and subdirectories (recursive):

```bash
./fixmd -r your_directory/
```

The tool will:

1.  Read the content of the Markdown file(s).
2.  Create a backup of each original file in a `backup` directory that mirrors the original directory structure.
3.  Send the content to Gemini AI for formatting and spelling correction.
4.  Replace the original file with the corrected version.

## Safety Features

- All files are backed up before any changes are made.
- Only processes Markdown files (`*.md`).
- Backups preserve the original directory structure.
- The backup directory is always created in the current working directory, not where the binary is located.


## System Requirements

- Go 1.21 or higher

## License

MIT
