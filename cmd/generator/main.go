package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-github/v45/github"
	"golang.org/x/oauth2"
)

const (
	orgName     = "blksails"
	baseDomain  = "pkg.blksails.net"
	basePackage = "pkg.blksails.net/pkg"
)

type PackageInfo struct {
	ImportPath string
	RepoURL    string
}

func main() {
	ctx := context.Background()

	// 使用 GitHub token 创建客户端
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// 获取组织下的所有仓库
	repos, _, err := client.Repositories.ListByOrg(ctx, orgName, nil)
	if err != nil {
		log.Fatalf("Error listing repositories: %v", err)
	}

	for _, repo := range repos {
		if repo.GetLanguage() != "Go" {
			continue
		}

		// 获取 go.mod 文件内容
		content, _, _, err := client.Repositories.GetContents(ctx, orgName, repo.GetName(), "go.mod", nil)
		if err != nil {
			continue
		}

		fileContent, err := content.GetContent()
		if err != nil {
			continue
		}

		// 解析 module 名称
		moduleName := parseModuleName(fileContent)
		if !strings.HasPrefix(moduleName, basePackage) {
			continue
		}

		// 生成对应的 HTML 文件
		pkgInfo := PackageInfo{
			ImportPath: moduleName,
			RepoURL:    repo.GetHTMLURL(),
		}

		if err := generateHTML(pkgInfo); err != nil {
			log.Printf("Error generating HTML for %s: %v", moduleName, err)
		}
	}
}

func parseModuleName(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "module ") {
			return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "module "))
		}
	}
	return ""
}

func generateHTML(pkg PackageInfo) error {
	tmpl := template.Must(template.New("index").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="go-import" content="{{ .ImportPath }} git {{ .RepoURL }}">
    <meta name="go-source" content="{{ .ImportPath }} {{ .RepoURL }} {{ .RepoURL }}/tree/master{/dir} {{ .RepoURL }}/blob/master{/dir}/{file}#L{line}">
    <meta http-equiv="refresh" content="0; url={{ .RepoURL }}">
</head>
<body>
    Redirecting to <a href="{{ .RepoURL }}">{{ .RepoURL }}</a>...
</body>
</html>`))

	// 创建目录结构
	relPath := strings.TrimPrefix(pkg.ImportPath, baseDomain+"/")
	dirPath := filepath.Join("public", relPath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// 创建 index.html 文件
	f, err := os.Create(filepath.Join(dirPath, "index.html"))
	if err != nil {
		return fmt.Errorf("failed to create file: %v", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, pkg); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	return nil
}
