# fixmd

A command-line tool that automatically formats and fixes spelling errors in Markdown files using the Gemini AI API.

## Features

- Formats and corrects spelling in Markdown files
- Processes individual files or entire directories
- Creates automatic backups with preserved directory structure
- Uses Google's Gemini AI for intelligent text processing

## Installation

1. Clone this repository
2. Build the application:

```bash
go build -o fixmd
```

3. Make it executable:

```bash
chmod +x fixmd
```

4. Optionally, move it to your PATH:

```bash
sudo mv fixmd /usr/local/bin/
```

5. Set up your environment variables by copying the sample file:

```bash
cp env.sample .env
```

Then edit `.env` to add your Gemini API key.

## Usage

Process a single file:
```bash
./fixmd your_file.md
```

Process all markdown files in a directory (non-recursive):
```bash
./fixmd your_directory/
```

Process all markdown files in a directory and subdirectories (recursive):
```bash
./fixmd -r your_directory/
```

The tool will:
1. Read the content of the markdown file(s)
2. Create a backup of each original file in a `backup` directory that mirrors the original directory structure
3. Send the content to Gemini AI for formatting and spelling correction
4. Replace the original file with the corrected version

## Safety Features

- All files are backed up before any changes are made
- Only processes markdown files (*.md)
- Backups preserve the original directory structure
- The backup directory is always created in the current working directory, not where the binary is located

## Configuration

Configuration is handled through environment variables or a `.env` file:

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| GEMINI_API_KEY | Your Google Gemini API key | Yes | - |
| GEMINI_API_URL | The Gemini API endpoint URL | No | https://generativelanguage.googleapis.com/v1beta/models/gemini-2.0-flash:generateContent |

You can get a Gemini API key from the [Google AI Studio](https://ai.google.dev/).

## System Requirements

- Go 1.21 or higher

## License

MIT
