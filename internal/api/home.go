package api

import "embed"

//go:embed static/home.html static/listener.html static/visual.html
var staticFiles embed.FS

func homePageHTML() (string, error) {
	content, err := staticFiles.ReadFile("static/listener.html")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func dashboardPageHTML() (string, error) {
	content, err := staticFiles.ReadFile("static/home.html")
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func visualPageHTML() (string, error) {
	content, err := staticFiles.ReadFile("static/visual.html")
	if err != nil {
		return "", err
	}
	return string(content), nil
}
