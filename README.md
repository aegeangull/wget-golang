# wget-golang

Wget is a free utility for non-interactive download of files from the Web. It supports HTTP, HTTPS, and FTP protocols, as well as retrieval through HTTP proxies.

This project objective consists on recreating some functionalities of wget using a compiled language. Golang in this case.

## Functionalities:

- The normal usage of wget: downloading a file given an URL, example: wget https://some_url.ogr/file.zip
- Downloading a single file and saving it under a different name
- Downloading and saving the file in a specific directory
- Set the download speed, limiting the rate speed of a download
- Downloading a file in background
- Downloading multiple files at the same time, by reading a file containing multiple download links asynchronously
- Main feature will be to download an entire website, mirroring a website

## Usage:
```
-B download in background
-O string
    output file name
-P string
    output directory (default ".")
-X string
    exclude directories (comma-separated)
-convert-links
    convert links for offline viewing
-i string
    file with URLs to download
-mirror
    mirror a website
-rate-limit string
    download rate limit (e.g., 200k, 2M)
-reject string
    reject file types (comma-separated)
```

### Check functionalities
- Run the script:
```
./run_commands.sh
```
Ctrl+C to quit

## Dependencies

The project requires the following Go modules:

- **golang.org/x/crypto**: v0.24.0
- **golang.org/x/net**: v0.26.0
- **github.com/mattn/go-isatty**: v0.0.20 (indirect)
- **github.com/k0kubun/go-ansi**: v0.0.0-20180517002512-3bf9e2903213
- **github.com/mitchellh/colorstring**: v0.0.0-20190213212951-d06e56a500db (indirect)
- **github.com/rivo/uniseg**: v0.4.7 (indirect)
- **github.com/schollz/progressbar/v3**: v3.14.4
- **golang.org/x/sys**: v0.21.0 (indirect)
- **golang.org/x/term**: v0.21.0 (indirect)

### Additional Indirect Dependencies

The following additional dependencies are also included indirectly:

- **github.com/davecgh/go-spew**: v1.1.1
- **github.com/pmezard/go-difflib**: v1.0.0
- **github.com/stretchr/objx**: v0.1.0
- **github.com/stretchr/testify**: v1.3.0

## Author

- [@aegeangull](https://github.com/aegeangull)

## License

MIT License