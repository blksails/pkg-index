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
	basePackage = "pkg.blksails.net"
)

type PackageInfo struct {
	ImportPath  string
	RepoURL     string
	Description string
}

func main() {
	ctx := context.Background()

	log.Printf("GITHUB_TOKEN: %s", os.Getenv("GITHUB_TOKEN"))
	// 使用 GitHub token 创建客户端
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// 获取组织下的所有仓库
	log.Printf("Fetching repositories for organization: %s", orgName)
	repos, _, err := client.Repositories.ListByOrg(ctx, orgName, nil)
	if err != nil {
		log.Fatalf("Error listing repositories: %v", err)
	}
	log.Printf("Found %d repositories", len(repos))

	var packages []PackageInfo

	for _, repo := range repos {
		log.Printf("Processing repository: %s", repo.GetName())
		if repo.GetLanguage() != "Go" {
			log.Printf("  Skipping %s: not a Go repository (language: %s)", repo.GetName(), repo.GetLanguage())
			continue
		}
		log.Printf("  Found Go repository: %s", repo.GetName())

		// Get repository contents recursively
		_, contents, _, err := client.Repositories.GetContents(ctx, orgName, repo.GetName(), "", nil)
		if err != nil {
			log.Printf("Error getting contents for %s: %v", repo.GetName(), err)
			continue
		}

		// Get go.mod file first to verify the module name
		log.Printf("  Checking go.mod for %s", repo.GetName())
		modContent, _, _, err := client.Repositories.GetContents(ctx, orgName, repo.GetName(), "go.mod", nil)
		if err != nil {
			log.Printf("  No go.mod found for %s, skipping", repo.GetName())
			continue
		}

		fileContent, err := modContent.GetContent()
		if err != nil {
			log.Printf("  Failed to read go.mod for %s: %v", repo.GetName(), err)
			continue
		}

		moduleName := parseModuleName(fileContent)
		log.Printf("  Module name: %s", moduleName)
		if !strings.HasPrefix(moduleName, basePackage) {
			log.Printf("  Skipping %s: module name doesn't start with %s", repo.GetName(), basePackage)
			continue
		}

		// 添加到包列表
		packages = append(packages, PackageInfo{
			ImportPath:  moduleName,
			RepoURL:     repo.GetHTMLURL(),
			Description: repo.GetDescription(),
		})

		// Generate HTML for main module
		log.Printf("  Generating HTML for main module: %s", moduleName)
		pkgInfo := PackageInfo{
			ImportPath:  moduleName,
			RepoURL:     repo.GetHTMLURL(),
			Description: repo.GetDescription(),
		}
		if err := generateHTML(pkgInfo); err != nil {
			log.Printf("  Error generating HTML for %s: %v", moduleName, err)
		} else {
			log.Printf("  ✓ Generated HTML for %s", moduleName)
		}

		// Process all Go files in subdirectories
		subPkgCount := 0
		for _, content := range contents {
			if content.GetType() == "file" && strings.HasSuffix(content.GetName(), ".go") {
				dir := filepath.Dir(content.GetPath())
				if dir == "." {
					continue // Skip root directory files as they're already handled
				}

				subPkgInfo := PackageInfo{
					ImportPath:  filepath.Join(moduleName, dir),
					RepoURL:     repo.GetHTMLURL(),
					Description: repo.GetDescription(),
				}
				if err := generateHTML(subPkgInfo); err != nil {
					log.Printf("  Error generating HTML for %s: %v", subPkgInfo.ImportPath, err)
				} else {
					log.Printf("  ✓ Generated HTML for subpackage: %s", subPkgInfo.ImportPath)
					subPkgCount++
				}
			}
		}
		if subPkgCount > 0 {
			log.Printf("  Generated %d subpackage(s) for %s", subPkgCount, repo.GetName())
		}
	}

	// 生成主页
	log.Printf("\nGenerating index HTML with %d package(s)", len(packages))
	if err := generateIndexHTML(packages); err != nil {
		log.Printf("Error generating index HTML: %v", err)
	} else {
		log.Printf("✓ Successfully generated index HTML")
	}

	log.Printf("\n=== Generation Complete ===")
	log.Printf("Total packages processed: %d", len(packages))
	log.Printf("Index page: public/index.html")
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

func generateIndexHTML(packages []PackageInfo) error {
	tmpl := template.Must(template.New("main-index").Parse(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>pkg.blksails.net</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, 'Open Sans', 'Helvetica Neue', sans-serif;
            max-width: 800px;
            margin: 0 auto;
            padding: 2rem;
            line-height: 1.6;
        }
        .package-list {
            margin-top: 2rem;
        }
        .package-item {
            margin-bottom: 1.5rem;
            padding: 1rem;
            border: 1px solid #eee;
            border-radius: 4px;
        }
        .package-item h3 {
            margin: 0 0 0.5rem 0;
        }
        .package-item p {
            margin: 0.5rem 0;
            color: #666;
        }
        code {
            background: #f5f5f5;
            padding: 0.2rem 0.4rem;
            border-radius: 3px;
            font-size: 0.9em;
        }
    </style>
</head>
<body>
    <h1>pkg.blksails.net</h1>
    <p>This is the package index for blksails Go packages.</p>
    <p>To use these packages in your Go project, simply import them using the <code>pkg.blksails.net/...</code>
        import path.</p>
    
    <div class="package-list">
        <h2>Available Packages</h2>
        {{range .}}
        <div class="package-item">
            <h3><a href="{{.RepoURL}}">{{.ImportPath}}</a></h3>
            {{if .Description}}
            <p>{{.Description}}</p>
            {{end}}
            <p><code>go get {{.ImportPath}}</code></p>
        </div>
        {{end}}
    </div>
</body>
</html>`))

	f, err := os.Create("public/index.html")
	if err != nil {
		return fmt.Errorf("failed to create index file: %v", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, packages); err != nil {
		return fmt.Errorf("failed to execute template: %v", err)
	}

	return nil
}
