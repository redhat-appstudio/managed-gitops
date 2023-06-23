package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/net/html"
)

// jobCmd represents the job command
var jobCmd = &cobra.Command{
	Use:   "job",
	Short: "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	Run: func(cmd *cobra.Command, args []string) {

		if len(args) == 0 {
			fmt.Println("* Error: missing URL argument")
			return
		} else if len(args) != 1 {
			fmt.Println("* Error: too many arguments")
			return
		}

		url := args[0]

		if !strings.HasPrefix(url, "https://") {
			fmt.Println("* Error: URL must begin with https://")
			return
		}

		fmt.Println("* Retrieving artifacts page")

		artifactsURL, err := downloadAndExtractArtifactsURL(url)
		if err != nil {
			fmt.Println("* Error:", err.Error())
			return
		}

		fmt.Println("* Creating list of all files within CI job")
		allFiles, err := extractAllFileURLsFromArtifactsPage(artifactsURL, 0)
		if err != nil {
			fmt.Println("* Error:", err)
			return
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
			pullConstantStr := "pull-ci-redhat-appstudio-managed-gitops-main-managed-gitops-e2e-tests"
			idx := strings.Index(filePath, pullConstantStr)
			if idx == -1 {
				pullConstantStr = "pull-ci-redhat-appstudio-infra-deployments-main-appstudio-e2e-tests"
				idx = strings.Index(filePath, pullConstantStr)
				if idx == -1 {
					fmt.Println("unexpected negative index in removing e2e-tests string constant:", filePath)
					return
				}
			}
			filePath = filePath[idx+len(pullConstantStr)+1:]

			// remove the job number
			idx = strings.Index(filePath, "/")
			if idx == -1 {
				fmt.Println("unexpected negative index in removing job number")
				return
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

			// downloadAsFile(fileURL, )
		}

		// for _, entryToDownload := range entriesToDownload {

		// 	fmt.Println(entryToDownload.path)
		// }

		fmt.Println("* Downloading", len(entriesToDownload), "files")

		downloadURLsMultithreaded(entriesToDownload)

		fmt.Println("* Download complete")
	},
}

const (
	debugEnabled = false
)

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

func skipTraverseURL(url string) bool {

	// various string constants which mean we don't need to both traversing the URL
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

func init() {
	downloadCmd.AddCommand(jobCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// jobCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// jobCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
