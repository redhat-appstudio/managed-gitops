package downloadjob

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/html"
)

const (
	debugEnabled = false
)

func RunDownloadJobCommand(url string) {

	if err := runDownloadJobCommandInternal(url); err != nil {
		fmt.Println("* Error:", err.Error())
		os.Exit(1)
		return
	}
}

func runDownloadJobCommandInternal(url string) error {

	fmt.Println("* Retrieving artifacts page")

	// I'm sure there's a much better way to parse this, but here we re.

	artifactsURL, err := downloadAndExtractArtifactsURL(url)
	if err != nil {
		return err
	}

	fmt.Println("* Creating list of all files within CI job")
	allFiles, err := extractAllFileURLsFromArtifactsPage(artifactsURL, 0)
	if err != nil {
		return err
	}

	entriesToDownload := []downloadURLWorkerEntry{}

	for _, fileURL := range allFiles {

		if skipFileDownload(fileURL) {
			continue
		}

		// remove https:// and host

		filePath := fileURL[8:]
		filePath = filePath[strings.Index(filePath, "/"):]

		// remove everything before this constant

		// Look for a known set of expects job names in the URL. New ones likely need to added here.
		validPullConstantStrs := []string{"pull-ci-redhat-appstudio-managed-gitops-main-managed-gitops-e2e-tests",
			"pull-ci-redhat-appstudio-infra-deployments-main-appstudio-e2e-tests"}

		var pullConstantStr string
		idx := -1
		for _, validPullConstantStr := range validPullConstantStrs {

			idx = strings.Index(filePath, validPullConstantStr)
			if idx != -1 {
				pullConstantStr = validPullConstantStr
				break
			}
		}

		if idx == -1 {
			return fmt.Errorf("unable to extract the pull constant string from the URL. This occurs when first run on a new repository. To fix this, add the pull-ci string to  'validPullConstantStrs'. path was: %s", filePath)
		}

		filePath = filePath[idx+len(pullConstantStr)+1:]

		// remove the job number
		idx = strings.Index(filePath, "/")
		if idx == -1 {
			return fmt.Errorf("unexpected negative index in removing job number. filepath was: %s", filePath)
		}
		filePath = filePath[idx+1:]

		// Don't try to download directories
		if strings.HasSuffix(filePath, "/") {
			continue
		}

		entriesToDownload = append(entriesToDownload, downloadURLWorkerEntry{
			url:  fileURL,
			path: "./downloaded/" + filePath,
		})

	}

	fmt.Println("* Downloading", len(entriesToDownload), "files")

	downloadURLsMultithreaded(entriesToDownload)

	fmt.Println("* Download complete")

	return nil
}

// skipFileDownload filters out URLs from download: returns true if a URL should not be downloaded, false otherwise
func skipFileDownload(url string) bool {

	// remove everything before, and including, '/artifacts/'
	{
		artifactsStrConst := "/artifacts/"

		index := strings.Index(url, artifactsStrConst)
		if index == -1 {
			return false
		}

		url = url[index+len("/artifacts/"):]
	}

	// remove everything before, and including, 'appstudio-e2e-tests'
	{
		e2eTestsConst := "appstudio-e2e-tests"

		e2eTestsIndex := strings.Index(url, e2eTestsConst)
		if e2eTestsIndex == -1 {
			return false
		}

		url = url[e2eTestsIndex+len(e2eTestsConst):]
	}

	if skipTraverseURL(url) {
		return true
	}

	if !(strings.Contains(url, "git") || strings.Contains(url, "appstudio")) {
		return true
	}

	return false

}

// skipTraverseURL will return true if we should avoid traversing (recursing scanning) a given URL, false otherwise.
func skipTraverseURL(url string) bool {

	// various string constants where we need not bother traversing the URL
	// - these are generally never going to be relevant to us in gitops service
	toSkip := []string{
		"jvm-build-service", "o11y", "quality-dashboard", "spi-system",
		"rp_preproc", "kube-rbac-proxy", "haproxy", "pipelines-as-code",
		"autoscaling", "enterprise-contract", "release-service", "tekton-results",
		"toolchain-host-operator", "dora-metrics", "spi-demos-", "tekton-chains",
		"spi-vault", "dev-sso", "kube-apiserver-proxy", "appstudio-grafana", "image-controller",
		"kube-public", "kam-", "toolchain-member-operator", "metrics", "kube-apiserver",
	}

	for _, toSkipEntry := range toSkip {

		if strings.Contains(url, toSkipEntry) {
			return true
		}

	}

	url = strings.ToLower(url)

	if strings.Contains(url, "/namespaces/") {

		if strings.Contains(url, "/namespaces/openshift-") && !strings.Contains(url, "git") {
			return true
		}

		if !(strings.Contains(url, "git") || strings.Contains(url, "appstudio")) {
			return true
		}

	}

	return false
}

func extractAllFileURLsFromArtifactsPage(url string, depth int) ([]string, error) {

	res := []string{}

	data, err := downloadURLAsString(url)
	if err != nil {
		return res, fmt.Errorf("%v", err)
	}

	doc, err := html.Parse(strings.NewReader(data))
	if err != nil {
		return res, fmt.Errorf("%v", err)
	}

	urlSuffixConstant := "openshiftapps.com/"

	host := url[:strings.Index(url, urlSuffixConstant)+len(urlSuffixConstant)-1]

	links := extractLinks(doc, 0)
	for index, link := range links {

		if index == 0 { // The first link is always a reference to the parent
			continue
		}

		if strings.Contains(link, "gsutil") {
			continue
		}

		newURL := host + link

		res = append(res, newURL)

		// Recurse into directories
		if strings.HasSuffix(newURL, "/") && !skipTraverseURL(newURL) {
			downloadRes, err := extractAllFileURLsFromArtifactsPage(newURL, depth+1)
			if err != nil {
				return nil, err
			}
			res = append(res, downloadRes...)
		}

	}

	return res, nil
}

func downloadAsFile(url string, path string) error {

	if debugEnabled {
		fmt.Println("* Downloading '" + url + "' to '" + path + "'")
	}

	parentDir := path[:strings.LastIndex(path, "/")]

	if err := os.MkdirAll(parentDir, os.ModePerm); err != nil {
		return fmt.Errorf("unable to create directory '%s': %v", parentDir, err)
	}

	data, err := downloadURLAsBytes(url)
	if err != nil {
		return fmt.Errorf("unable to download URL '%s' as bytes: %v", url, err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("unable to create path '%s': %v", path, err)
	}

	_, err = file.Write(data)
	if err != nil {
		return fmt.Errorf("unable to write data: %v", err)
	}

	return nil

}

func downloadAndExtractArtifactsURL(url string) (string, error) {
	data, err := downloadURLAsString(url)
	if err != nil {
		return "", fmt.Errorf("%v", err)
	}

	doc, err := html.Parse(strings.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("%v", err)
	}

	var artifactsURL string

	urls := extractLinks(doc, 0)

	for _, url := range urls {
		if strings.Contains(url, "gcsweb-ci") {
			artifactsURL = url
		}
	}

	if artifactsURL == "" {
		return "", fmt.Errorf("* Error: unable to extract artifacts URL: %v", err)
	}

	return artifactsURL, nil
}

// extractArtifactsURL parses the HTML and extracts the link to the Artifacts
func extractLinks(node *html.Node, depth int) []string {

	res := []string{}

	for c := node.FirstChild; c != nil; c = c.NextSibling {

		res = append(res, extractLinks(c, depth+1)...)

		if c.Data != "a" {
			continue
		}

		for _, addr := range c.Attr {
			res = append(res, addr.Val)
		}

	}

	return res

}

func downloadURLAsString(webPage string) (string, error) {

	resp, err := http.Get(webPage)

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return "", err
	}

	return string(body), nil
}

func downloadURLAsBytes(url string) ([]byte, error) {

	resp, err := http.Get(url)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	return body, nil
}
