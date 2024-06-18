package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
)

// convertHTMLToMarkdownはHTML文字列をMarkdown文字列に変換します
func convertHTMLToMarkdown(htmlContent string) string {
	converter := md.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(htmlContent)
	if err != nil {
		log.Fatal(err)
	}
	return markdown
}

// processFileは単一のHTMLファイルをMarkdownに変換して元のディレクトリ構成を維持しつつ保存します
func processFile(inputPath, baseDir, outputDir string) {
	// 入力ファイルの相対パスを取得
	relPath, _ := filepath.Rel(baseDir, inputPath)
	// 出力ファイルのパスを生成
	outputPath := filepath.Join(outputDir, strings.TrimSuffix(relPath, filepath.Ext(relPath))+".md")

	// 出力ディレクトリを作成
	outputFileDir := filepath.Dir(outputPath)
	os.MkdirAll(outputFileDir, 0755)

	// 入力HTMLファイルを読み込み
	htmlContent, err := ioutil.ReadFile(inputPath)
	if err != nil {
		panic(fmt.Sprintf("Error reading file %s: %v", inputPath, err))
	}

	// HTMLをMarkdownに変換
	mdContent := convertHTMLToMarkdown(string(htmlContent))

	// Markdownを出力ファイルに書き込み
	err = ioutil.WriteFile(outputPath, []byte(mdContent), 0644)
	if err != nil {
		panic(fmt.Sprintf("Error writing file %s: %v", outputPath, err))
	}

	fmt.Printf("Converted %s to %s\n", inputPath, outputPath)
}

// copyFileはHTML以外のファイルを元のディレクトリ構成を維持しつつコピーします
func copyFile(inputPath, baseDir, outputDir string) {
	// 入力ファイルの相対パスを取得
	relPath, _ := filepath.Rel(baseDir, inputPath)
	// 出力ファイルのパスを生成
	outputPath := filepath.Join(outputDir, relPath)

	// 出力ディレクトリを作成
	outputFileDir := filepath.Dir(outputPath)
	os.MkdirAll(outputFileDir, 0755)

	// ファイルをコピー
	input, err := ioutil.ReadFile(inputPath)
	if err != nil {
		panic(fmt.Sprintf("Error reading file %s: %v", inputPath, err))
	}
	err = ioutil.WriteFile(outputPath, input, 0644)
	if err != nil {
		panic(fmt.Sprintf("Error writing file %s: %v", outputPath, err))
	}

	fmt.Printf("Copied %s to %s\n", inputPath, outputPath)
}

// processDirectoryはディレクトリ内の全てのファイルを再帰的に処理します
func processDirectory(directory, outputDir string) {
	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
				processFile(path, directory, outputDir)
			} else {
				copyFile(path, directory, outputDir)
			}
		}
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("Error walking the path %v: %v", directory, err))
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: html2md <path_to_html_or_directory1> <path_to_html_or_directory2> ...")
		return
	}

	// outputディレクトリを作成
	outputDir := "output"
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		panic(fmt.Sprintf("Error creating output directory: %v", err))
	}

	for _, inputPath := range os.Args[1:] {
		info, err := os.Stat(inputPath)
		if err != nil {
			panic(fmt.Sprintf("Error stating file or directory %s: %v", inputPath, err))
		}

		if info.IsDir() {
			// ディレクトリの場合、再帰的に処理
			processDirectory(inputPath, outputDir)
		} else if strings.HasSuffix(strings.ToLower(info.Name()), ".html") {
			// HTMLファイルの場合、処理
			processFile(inputPath, filepath.Dir(inputPath), outputDir)
		} else {
			// HTML以外のファイルの場合、コピー
			copyFile(inputPath, filepath.Dir(inputPath), outputDir)
		}
	}

	fmt.Println("Conversion and copying completed.")
}
