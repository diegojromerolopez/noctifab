# Markdown-to-HTML Converter Specification

## 1. Overview
The goal of this project is to implement a command-line interface (CLI) tool that parses a markdown file containing basic markdown formatting (headings, paragraphs, bold text, links, and unordered lists) and compiles it into a clean, valid HTML file.

---

## 2. Requirements

### 2.1. CLI Arguments
The program must support the following command-line flags:

*   `-i`, `--input`: (Required) Path to the source `.md` file.
*   `-o`, `--output`: (Required) Path to the target `.html` file.
*   `-t`, `--template`: (Optional) Path to a template HTML file. If provided, the parsed HTML body should replace a placeholder (e.g. `{{content}}`) in the template. If not provided, wrap the parsed content in a basic HTML boilerplate.

### 2.2. Supported Markdown Syntax
The parser must support at least the following subset of Markdown:

1.  **Headings:**
    *   `# Heading 1` -> `<h1>Heading 1</h1>`
    *   `## Heading 2` -> `<h2>Heading 2</h2>`
    *   `### Heading 3` -> `<h3>Heading 3</h3>`
2.  **Paragraphs:**
    *   Blocks of text separated by blank lines (double newlines) should be wrapped in `<p>...</p>`.
3.  **Bold Text:**
    *   `**bold text**` or `__bold text__` -> `<strong>bold text</strong>`
4.  **Links:**
    *   `[Google](https://google.com)` -> `<a href="https://google.com">Google</a>`
5.  **Unordered Lists:**
    *   Lines starting with `- ` or `* ` should be grouped into `<ul>` and `<li>` elements.
        ```markdown
        - Item A
        - Item B
        ```
        becomes:
        ```html
        <ul>
          <li>Item A</li>
          <li>Item B</li>
        </ul>
        ```

### 2.3. Error Handling
*   If the input file does not exist, the program must print a clear error message to `stderr` and exit with code `1`.
*   If the output file cannot be written (e.g. directory permission issue), print an error to `stderr` and exit with code `2`.

---

## 3. Technical Constraints
*   **Language:** Go, Python, or Node.js.
*   **Dependencies:** Standard libraries are preferred. Rather than pulling in large third-party Markdown libraries (like `blackfriday` or `marked`), implementing a simple custom line-by-line regex-based parser is recommended to validate the agent's parsing logic.

---

## 4. Verification Criteria & Testing

### 4.1. Expected Tests
*   **Unit Tests:**
    *   Verify individual translation rules (e.g. bold regex replacement, headings parsing).
    *   Verify list aggregation logic (correct nesting of `<li>` inside `<ul>` without duplicate tags).
*   **Integration Tests:**
    *   Write a sample markdown file to a temp directory, execute the converter, and check the generated HTML output files against expected HTML content.
    *   Verify template replacement logic when `-t` is provided.
    *   Verify correct error output and exit status for missing files.
