package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/schollz/progressbar/v3"
	"github.com/k0kubun/go-ansi"
	"golang.org/x/net/html"
	"golang.org/x/crypto/ssh/terminal"
)

func main() {
	outputName := flag.String("O", "", "output file name")
	outputPath := flag.String("P", ".", "output directory")
	rateLimit := flag.String("rate-limit", "", "download rate limit (e.g., 200k, 2M)")
	inputFile := flag.String("i", "", "file with URLs to download")
	mirror := flag.Bool("mirror", false, "mirror a website")
	exclude := flag.String("X", "", "exclude directories (comma-separated)")
	reject := flag.String("reject", "", "reject file types (comma-separated)")
	convertLinks := flag.Bool("convert-links", false, "convert links for offline viewing")
	background := flag.Bool("B", false, "download in background")

	flag.Parse()

	url := flag.Arg(0)
	if url == "" && *inputFile == "" && !*mirror {
		fmt.Println("URL or input file is required")
		return
	}

	bps := parseRateLimit(*rateLimit)

	if *background {
		logFile, err := os.Create("wget-log")
		if err != nil {
			fmt.Println("Error creating log file:", err)
			return
		}
		defer logFile.Close()
		fmt.Println("Output will be written to 'wget-log'")

		var wg sync.WaitGroup

		if *mirror && url != "" {
			rejectList := getRejectList(strings.Split(*reject, ","))
			excludeList := getExcludeList(strings.Split(*exclude, ","))
			wg.Add(1)
			go mirrorWebsite(url, getRejectList(rejectList), getExcludeList(excludeList), *convertLinks, logFile, &wg)
		} else if url != "" {
			wg.Add(1)
			go downloadFile(url, filepath.Join(*outputPath, filepath.Base(url)), bps, logFile, &wg, true)
		} else if *inputFile != "" {
			go downloadFilesFromList(*inputFile, bps, logFile, &wg, true)
		}

		wg.Wait()
		return
	}

	var wg sync.WaitGroup

	if *mirror && url != "" {
		rejectList := getRejectList(strings.Split(*reject, ","))
		excludeList := getExcludeList(strings.Split(*exclude, ","))
		wg.Add(1)
		mirrorWebsite(url, rejectList, excludeList, *convertLinks, os.Stdout, &wg)
		wg.Wait()
	} else if url != "" {
		outputFileName := *outputName
		if outputFileName == "" {
			outputFileName = filepath.Base(url)
		}
		outputFilePath := filepath.Join(*outputPath, outputFileName)
		wg.Add(1)
		downloadFile(url, outputFilePath, bps, os.Stdout, &wg, false)
		wg.Wait()
	} else if *inputFile != "" {
		wg.Add(1)
		err := downloadFilesFromList(*inputFile, bps, os.Stdout, &wg, false)
		wg.Wait()
		if err != nil {
			fmt.Println("Error downloading files from list:", err)
		}
	}
}

func getTerminalWidth() int {
	width, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Default to 80 if unable to get terminal size
		return 80
	}
	return width
}

// ThrottledReader limits the read speed from the underlying reader
type ThrottledReader struct {
	r        io.Reader
	bps      int // bytes per second
	bytes    int
	lastTick time.Time
}

func (tr *ThrottledReader) Read(p []byte) (int, error) {
	// Calculate time elapsed
	now := time.Now()
	elapsed := now.Sub(tr.lastTick).Seconds()

	// Calculate the max bytes we can read in this elapsed time
	maxBytes := int(float64(tr.bps) * elapsed)
	if maxBytes <= 0 {
		// If no time has passed, sleep for a bit
		time.Sleep(time.Millisecond * 10)
		return tr.Read(p)
	}

	// Read from the underlying reader
	if maxBytes < len(p) {
		p = p[:maxBytes]
	}

	n, err := tr.r.Read(p)
	tr.bytes += n
	tr.lastTick = now

	return n, err
}

func newThrottledReader(r io.Reader, bps int) *ThrottledReader {
	return &ThrottledReader{
		r:        r,
		bps:      bps,
		lastTick: time.Now(),
	}
}

func downloadFile(url, outputPath string, bps int, log *os.File, wg *sync.WaitGroup, silent bool) {
	defer wg.Done()

	startTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(log, "start at %s\n", startTime)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Fprintf(log, "error: %v\n", err)
		return
	}
	req.Header.Set("User-Agent", "Wget/1.21.1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(log, "error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(log, "status %s\n", resp.Status)
		return
	}
	fmt.Fprintf(log, "status %s\n", resp.Status)

	contentLength := resp.ContentLength
	fmt.Fprintf(log, "content size: %d bytes [%.2f MB]\n", contentLength, float64(contentLength)/(1024*1024))

	err = os.MkdirAll(filepath.Dir(outputPath), os.ModePerm)
	if err != nil {
		fmt.Fprintf(log, "error creating directories: %v\n", err)
		return
	}

	file, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(log, "error: %v\n", err)
		return
	}
	defer file.Close()

	fmt.Fprintf(log, "saving file to: %s\n", outputPath)

	var reader io.Reader = resp.Body
	if bps > 0 {
		reader = newThrottledReader(resp.Body, bps)
	}

	if !silent {
		bar := configureProgressBar(contentLength)
		defer bar.Close()

		writer := io.MultiWriter(file, bar)
		n, err := io.Copy(writer, reader)
		if err != nil {
			fmt.Fprintf(log, "error: %v\n", err)
			return
		}
		fmt.Fprintf(log, "\nDownloaded [%s]\n", url)
		fmt.Fprintf(log, "finished at %s\n", time.Now().Format("2006-01-02 15:04:05"))
		fmt.Fprintf(log, "total size: %d bytes\n", n)
	} else {
		n, err := io.Copy(file, reader)
		if err != nil {
			fmt.Fprintf(log, "error: %v\n", err)
			return
		}
		fmt.Fprintf(log, "\nDownloaded [%s]\n", url)
		fmt.Fprintf(log, "finished at %s\n", time.Now().Format("2006-01-02 15:04:05"))
		fmt.Fprintf(log, "total size: %d bytes\n", n)
	}
}

func configureProgressBar(total int64) *progressbar.ProgressBar {
	termWidth := getTerminalWidth()
	return progressbar.NewOptions64(total,
		progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
		progressbar.OptionEnableColorCodes(true),
		progressbar.OptionSetWidth(termWidth-70),
		progressbar.OptionSetDescription("[cyan]downloading"),
		progressbar.OptionShowCount(),
		progressbar.OptionShowIts(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionThrottle(65*time.Millisecond),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        " ",
			AltSaucerHead: "[yellow](\"<[reset]",
			SaucerHead:    "[yellow](\"-[reset]",
			SaucerPadding: "[white]•",
			BarStart:      "[blue]|[reset]",
			BarEnd:        "[blue]|[reset]",
		}),
	)
}

func parseRateLimit(rateLimit string) int {
	if rateLimit == "" {
		return 0
	}

	bps := 0
	if strings.HasSuffix(rateLimit, "k") {
		fmt.Sscanf(rateLimit, "%dk", &bps)
		bps *= 1024
	} else if strings.HasSuffix(rateLimit, "M") {
		fmt.Sscanf(rateLimit, "%dM", &bps)
		bps *= 1024 * 1024
	} else {
		fmt.Sscanf(rateLimit, "%d", &bps)
	}

	return bps
}

func downloadFilesFromList(inputFile string, bps int, log *os.File, wg *sync.WaitGroup, silent bool) error {
	defer wg.Done()

	file, err := os.Open(inputFile)
	if err != nil {
		return fmt.Errorf("error opening input file: %v", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var urls []string
	for scanner.Scan() {
		urls = append(urls, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading input file: %v", err)
	}

	var downloadWg sync.WaitGroup

	for _, url := range urls {
		outputFileName := filepath.Base(url)
		outputFilePath := filepath.Join(".", outputFileName)
		downloadWg.Add(1)
		go downloadFile(url, outputFilePath, bps, log, &downloadWg, silent)
	}

	downloadWg.Wait()

	return nil
}

func downloadResource(url, outputPath string, log *os.File, rejectList, excludeList []string) error {
	if rejectList != nil && shouldReject(url, rejectList) {
		fmt.Fprintf(log, "skipping download of %s due to reject list\n", url)
		return nil
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Wget/1.21.1")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("error downloading resource: %s", resp.Status)
	}

	if excludeList != nil && shouldExclude(filepath.Dir(outputPath), excludeList) {
		fmt.Fprintf(log, "skipping directory %s due to exclude list\n", filepath.Dir(outputPath))
		return nil
	}
	err = os.MkdirAll(filepath.Dir(outputPath), os.ModePerm)
	if err != nil {
		return err
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

var cssURLRegex = regexp.MustCompile(`url\(['"]?([^'"]+?)['"]?\)`)

func extractURLsFromCSS(css string) []string {
	matches := cssURLRegex.FindAllStringSubmatch(css, -1)
	urls := make([]string, len(matches))
	for i, match := range matches {
		urls[i] = match[1]
	}
	return urls
}

func mirrorWebsite(url string, rejectList, excludeList []string, convertLinks bool, log *os.File, wg *sync.WaitGroup) {
	defer wg.Done()

	startTime := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(log, "start at %s\n", startTime)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Fprintf(log, "error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(log, "status %s\n", resp.Status)
		return
	}

	doc, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(log, "error: %v\n", err)
		return
	}

	baseURL := strings.TrimSuffix(url, "/")
	outputDir := filepath.Join(".", strings.ReplaceAll(filepath.Base(baseURL), ".", "_"))

	excludeList = getExcludeList(excludeList) // Ensure excludeList is processed here
	
	if excludeList != nil && shouldExclude(outputDir, excludeList) {
		fmt.Fprintf(log, "skipping directory %s due to exclude list\n", outputDir)
		return
	}
	err = os.MkdirAll(outputDir, os.ModePerm)
	if err != nil {
		fmt.Fprintf(log, "error creating directory %s: %v\n", outputDir, err)
		return
	}

	outputFilePath := filepath.Join(outputDir, "index.html")
	err = os.WriteFile(outputFilePath, doc, 0644)
	if err != nil {
		fmt.Fprintf(log, "error writing to file %s: %v\n", outputFilePath, err)
		return
	}

	node, err := html.Parse(strings.NewReader(string(doc)))
	if err != nil {
		fmt.Fprintf(log, "error parsing HTML: %v\n", err)
		return
	}

	var wgResources sync.WaitGroup

	var traverse func(*html.Node)
	traverse = func(n *html.Node) {
		if n.Type == html.ElementNode {
			var attrKey, attrValue, outputPath string

			switch n.Data {
			case "link":
				for _, a := range n.Attr {
					if a.Key == "rel" && a.Val == "stylesheet" {
						for _, a := range n.Attr {
							if a.Key == "href" {
								attrKey, attrValue = "href", a.Val
								break
							}
						}
					}
				}
			case "img":
				for _, a := range n.Attr {
					if a.Key == "src" {
						attrKey, attrValue = "src", a.Val
						break
					}
				}
			case "script":
				for _, a := range n.Attr {
					if a.Key == "src" {
						attrKey, attrValue = "src", a.Val
						break
					}
				}
			case "style":
				// Process CSS content inside <style> tags
				if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					cssContent := n.FirstChild.Data
					urls := extractURLsFromCSS(cssContent)

					for _, resourceURL := range urls {
						if rejectList != nil && shouldReject(resourceURL, rejectList) {
							fmt.Fprintf(log, "skipping download of %s due to reject list\n", resourceURL)
							continue
						}
						if !strings.HasPrefix(resourceURL, "http") {
							resourceURL = resolveURL(baseURL, resourceURL)
						}
						outputPath = filepath.Join(outputDir, "img", filepath.Base(resourceURL))

						wgResources.Add(1)
						go func(url, path string) {
							defer wgResources.Done()
							err := downloadResource(url, path, log, rejectList, excludeList)
							if err != nil {
								fmt.Fprintf(log, "error downloading resource %s: %v\n", url, err)
							}
						}(resourceURL, outputPath)

						relativePath := filepath.Join("..", "img", filepath.Base(resourceURL))
						cssContent = strings.ReplaceAll(cssContent, resourceURL, relativePath)
					}

					// Update the CSS content inside the <style> tag
					n.FirstChild.Data = cssContent
				}
			}

			if attrKey != "" && attrValue != "" {
				if rejectList != nil && shouldReject(attrValue, rejectList) {
					fmt.Fprintf(log, "skipping download of %s due to reject list\n", attrValue)
					return
				}
				if !strings.HasPrefix(attrValue, "http") {
					// Resolve the relative URL to an absolute URL
					attrValue = resolveURL(baseURL, attrValue)
				}
				resourceURL := attrValue
				resourcePath := attrValue[len(url):]
				if n.Data == "img" {
					outputPath = filepath.Join(outputDir, "img", filepath.Base(resourcePath))
				} else if n.Data == "link" {
					outputPath = filepath.Join(outputDir, "css", filepath.Base(resourcePath))
				} else {
					outputPath = filepath.Join(outputDir, filepath.Dir(resourcePath), filepath.Base(resourcePath))
				}

				excludeList = getExcludeList(excludeList) // Ensure excludeList is processed here

				// Create necessary directories
				if excludeList != nil && shouldExclude(filepath.Dir(outputPath), excludeList) {
					fmt.Fprintf(log, "skipping directory %s due to exclude list\n", filepath.Dir(outputPath))
					return
				}
				err := os.MkdirAll(filepath.Dir(outputPath), os.ModePerm)
				if err != nil {
					fmt.Fprintf(log, "error creating directories for %s: %v\n", outputPath, err)
					return
				}

				rejectList = getRejectList(rejectList) // Ensure rejectList is processed here

				if rejectList != nil && shouldReject(resourceURL, rejectList) {
					fmt.Fprintf(log, "skipping download of %s due to reject list\n", resourceURL)
					return
				}

				wgResources.Add(1)
				go func(url, path string) {
					defer wgResources.Done()
					err := downloadResource(url, path, log, rejectList, excludeList)
					if err != nil {
						fmt.Fprintf(log, "error downloading resource %s: %v\n", url, err)
					}
				}(resourceURL, outputPath)

				for i, a := range n.Attr {
					if a.Key == attrKey {
						n.Attr[i].Val = filepath.Join(filepath.Dir(outputPath[len(outputDir)+1:]), filepath.Base(outputPath))
					}
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			traverse(c)
		}
	}

	traverse(node)
	wgResources.Wait()

	// Update the CSS files
	updateCSSFiles(node, url, outputDir, log, rejectList, excludeList, &wgResources)
	wgResources.Wait()

	var buf strings.Builder
	html.Render(&buf, node)
	err = os.WriteFile(outputFilePath, []byte(buf.String()), 0644)
	if err != nil {
		fmt.Fprintf(log, "error: %v\n", err)
		return
	}

	fmt.Fprintf(log, "Mirrored %s to %s\n", url, outputFilePath)
	fmt.Fprintf(log, "finished at %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

func resolveURL(baseURL, relativePath string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return relativePath
	}
	ref, err := url.Parse(relativePath)
	if err != nil {
		return relativePath
	}
	return u.ResolveReference(ref).String()
}

func updateCSSFiles(node *html.Node, baseURL, outputDir string, log *os.File, rejectList, excludeList []string, wg *sync.WaitGroup) {
	if node.Type == html.ElementNode && node.Data == "link" {
		for _, a := range node.Attr {
			if a.Key == "rel" && a.Val == "stylesheet" {
				for _, a := range node.Attr {
					if a.Key == "href" {
						cssURL := a.Val
						if !strings.HasPrefix(cssURL, "http") {
							cssURL = baseURL + "/" + strings.TrimLeft(cssURL, "/")
						}
						outputPath := filepath.Join(outputDir, "css", filepath.Base(cssURL))
						wg.Add(1)
						go func(url, path string) {
							defer wg.Done()
							err := downloadResource(url, path, log, rejectList, excludeList)
							if err != nil {
								fmt.Fprintf(log, "error downloading CSS file %s: %v\n", url, err)
								return
							}
							err = processCSSFile(path, baseURL, outputDir, log, rejectList, excludeList, wg)
							if err != nil {
								fmt.Fprintf(log, "error processing CSS file %s: %v\n", path, err)
							}
						}(cssURL, outputPath)
					}
				}
			}
		}
	}else if node.Type == html.ElementNode && node.Data == "style" {
		if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
			cssContent := node.FirstChild.Data
			urls := extractURLsFromCSS(cssContent)

			for _, url := range urls {
				absoluteURL := url
				if !strings.HasPrefix(url, "http") {
					absoluteURL = baseURL + "/" + strings.TrimLeft(url, "/")
				}

				outputPath := filepath.Join(outputDir, "img", filepath.Base(url))
				wg.Add(1)
				go func(url, path string) {
					defer wg.Done()
					err := downloadResource(url, path, log, rejectList, excludeList)
					if err != nil {
						fmt.Fprintf(log, "error downloading CSS resource %s: %v\n", url, err)
					}
				}(absoluteURL, outputPath)

				relativePath := filepath.Join("img", filepath.Base(url))
				cssContent = strings.ReplaceAll(cssContent, url, relativePath)
			}

			// Update the CSS content inside the <style> tag
			node.FirstChild.Data = cssContent
		}
	}
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		updateCSSFiles(c, baseURL, outputDir, log, rejectList, excludeList, wg)
	}
}

func processCSSFile(filePath, baseURL, outputDir string, log *os.File, rejectList, excludeList []string, wg *sync.WaitGroup) error {
	cssData, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	cssContent := string(cssData)
	urls := extractURLsFromCSS(cssContent)

	for _, url := range urls {
		absoluteURL := url
		if !strings.HasPrefix(url, "http") {
			absoluteURL = baseURL + "/" + strings.TrimLeft(url, "/")
		}

		outputPath := filepath.Join(outputDir, "img", filepath.Base(url))
		wg.Add(1)
		go func(url, path string) {
			defer wg.Done()
			err := downloadResource(url, path, log, rejectList, excludeList)
			if err != nil {
				fmt.Fprintf(log, "error downloading CSS resource %s: %v\n", url, err)
			}
		}(absoluteURL, outputPath)

		relativePath := filepath.Join("..", "img", filepath.Base(url))
		cssContent = strings.ReplaceAll(cssContent, url, relativePath)
	}

	err = os.WriteFile(filePath, []byte(cssContent), 0644)
	if err != nil {
		return err
	}

	return nil
}

func shouldReject(url string, rejectList []string) bool {
	for _, ext := range rejectList {
		if strings.HasSuffix(url, ext) {
			return true
		}
	}
	return false
}

func getRejectList(rejectList []string) []string {
	// If the reject list is empty or contains only empty strings, return nil
	if len(rejectList) == 0 || (len(rejectList) == 1 && rejectList[0] == "") {
		return nil
	}
	return rejectList
}

func shouldExclude(path string, excludeList []string) bool {
    for _, exclude := range excludeList {
        if strings.Contains(path, exclude) {
            return true
        }
    }
    return false
}

func getExcludeList(excludeList []string) []string {
	// If the exclude list is empty or contains only empty strings, return nil
	if len(excludeList) == 0 || (len(excludeList) == 1 && excludeList[0] == "") {
		return nil
	}
	return excludeList
}
