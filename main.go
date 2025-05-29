package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"

	md "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/alexflint/go-arg"
	"github.com/pkg/errors"
)

//! 引数を管理する構造体。
type Args struct {
	InputDir     string `arg:"positional,required" help:"変換対象のディレクトリパス"`
	Suffix       string `arg:"-s,--suffix" default:"_converted" help:"出力ディレクトリのサフィックス"`
	RenamePrefix string `arg:"--rename-prefix" default:"_" help:"元のHTMLファイル名に付与するプレフィックス"`
}

//! ディレクトリエントリを表す構造体。
type DirEntry struct {
	Name     string      // ファイル名またはディレクトリ名。
	Path     string      // 相対パス。
	IsDir    bool        // ディレクトリかどうか。
	Children []*DirEntry // 子要素(ディレクトリの場合)。
}

// グローバル変数。
var (
	args   Args
	parser *arg.Parser // ShowHelp() で使う

	version  string = "debug build"   // makefileからビルドされると上書きされる。
	revision string = func() string { // {{{
		revision := ""
		modified := false
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					//return setting.Value
					revision = setting.Value
					if len(setting.Value) > 7 {
						revision = setting.Value[:7] // 最初の7文字にする
					}
				}
				if setting.Key == "vcs.modified" {
					modified = setting.Value == "true"
				}
			}
		}
		if modified {
			revision = "develop+" + revision
		}
		return revision
	}() // }}}
)

//! 初期化処理でログ設定を行う。
func init() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lshortfile)
}

//! メイン関数。引数解析後に変換処理を実行する。
func main() {
	ParseArgs()
	err := ConvertHtmlToMarkdown()
	if err != nil {
		panic(errors.Errorf("変換処理に失敗しました: %v", err))
	}
}

func (Args) Version() string {
	return GetVersion()
}

func ShowHelp(post string) {
	buf := new(bytes.Buffer)
	parser.WriteHelp(buf)
	help := buf.String()
	help = strings.ReplaceAll(help, "display this help and exit", "ヘルプを出力する。")
	help = strings.ReplaceAll(help, "display version and exit", "バージョンを出力する。")
	fmt.Printf("%v\n", help)
	if len(post) != 0 {
		fmt.Println(post)
	}
	os.Exit(1)
}

func GetFileNameWithoutExt(path string) string {
	return filepath.Base(path[:len(path)-len(filepath.Ext(path))])
}

func GetVersion() string {
	if len(revision) == 0 {
		// go installでビルドされた場合、gitの情報がなくなる。その場合v0.0.0.のように末尾に.がついてしまうのを避ける。
		return fmt.Sprintf("%v version %v", GetFileNameWithoutExt(os.Args[0]), version)
	} else {
		return fmt.Sprintf("%v version %v.%v", GetFileNameWithoutExt(os.Args[0]), version, revision)
	}
}

func ShowVersion() {
	fmt.Printf("%s\n", GetVersion())
	os.Exit(0)
}

//! go-argを使用して引数を解析する。
func ParseArgs() {
	var err error
	parser, err = arg.NewParser(arg.Config{Program: GetFileNameWithoutExt(os.Args[0]), IgnoreEnv: false}, &args)
	if err != nil {
		ShowHelp(fmt.Sprintf("%v", errors.Errorf("%v", err)))
		os.Exit(1)
	}

	err = parser.Parse(os.Args[1:])
	if err != nil {
		if err.Error() == "help requested by user" {
			ShowHelp("")
			os.Exit(1)
		} else if err.Error() == "version requested by user" {
			ShowVersion()
			os.Exit(0)
		} else if strings.Contains(err.Error(), "unknown argument") {
			fmt.Printf("%s\n", errors.Errorf("%v", err))
			os.Exit(1)
		} else {
			panic(errors.Errorf("%v", err))
		}
	}
}

//! HTML→Markdown変換のメイン処理を行う。
func ConvertHtmlToMarkdown() error {
	// 入力ディレクトリの存在確認。
	if _, err := os.Stat(args.InputDir); os.IsNotExist(err) {
		return errors.Errorf("入力ディレクトリが存在しません: %s", args.InputDir)
	}

	// 出力ディレクトリ名を生成。
	// 入力パスの親ディレクトリと基底名を分離。
	inputDir := filepath.Clean(args.InputDir)
	parentDir := filepath.Dir(inputDir)
	baseName := filepath.Base(inputDir)
	
	// 出力ディレクトリは親ディレクトリ直下に作成。
	outputDir := filepath.Join(parentDir, baseName+args.Suffix)
	
	// 出力ディレクトリが存在しない場合のみ作成。
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return errors.Errorf("出力ディレクトリの作成に失敗: %v", err)
		}
		
		// ディレクトリ全体をコピー。
		if err := CopyDirectory(args.InputDir, outputDir); err != nil {
			return errors.Errorf("ディレクトリコピーに失敗: %v", err)
		}
	} else {
		log.Printf("出力ディレクトリが既に存在します: %s", outputDir)
	}

	// HTMLファイルを変換。
	log.Printf("HTMLファイル変換を開始します...")
	if err := ProcessHtmlFiles(outputDir); err != nil {
		return errors.Errorf("HTMLファイル変換に失敗: %v", err)
	}
	
	// HTMLファイルをリネーム。
	log.Printf("HTMLファイルリネームを開始します...")
	if err := RenameHtmlFiles(outputDir); err != nil {
		return errors.Errorf("HTMLファイルリネームに失敗: %v", err)
	}
	
	// ディレクトリ名を小文字にリネーム。
	log.Printf("ディレクトリ名小文字化を開始します...")
	if err := RenameDirectoriesToLowercase(outputDir); err != nil {
		return errors.Errorf("ディレクトリ名小文字化に失敗: %v", err)
	}

	// mdbook用ファイル生成。
	log.Printf("mdbook用ファイル生成を開始します...")
	if err := GenerateMdBookFiles(outputDir); err != nil {
		return errors.Errorf("mdbook用ファイル生成に失敗: %v", err)
	}
	
	fmt.Printf("変換完了: %s → %s\n", args.InputDir, outputDir)
	return nil
}

//! ディレクトリを再帰的にコピーする。
func CopyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 相対パスを計算。
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			// ディレクトリを作成。
			return os.MkdirAll(dstPath, info.Mode())
		} else {
			// ファイルをコピー。
			return CopyFile(path, dstPath)
		}
	})
}

//! 単一ファイルをコピーする。
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// 出力ファイルのディレクトリを作成。
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

//! 出力ディレクトリ内のHTMLファイルを処理する。
func ProcessHtmlFiles(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// HTMLファイルのみを対象とする。
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".html") {
			return ConvertSingleHtmlFile(path)
		}
		return nil
	})
}

//! 単一のHTMLファイルをMarkdownに変換する。
func ConvertSingleHtmlFile(htmlPath string) error {
	log.Printf("変換中: %s", htmlPath)

	// HTMLファイルを読み込み。
	htmlContent, err := os.ReadFile(htmlPath)
	if err != nil {
		return errors.Errorf("HTMLファイル読み込みエラー: %v", err)
	}

	// html-to-markdownコンバーターを作成。
	converter := md.NewConverter("", true, nil)
	
	// HTMLをMarkdownに変換。
	markdownContent, err := converter.ConvertString(string(htmlContent))
	if err != nil {
		return errors.Errorf("HTML→Markdown変換エラー: %v", err)
	}

	// HTMLへの相対リンクをMarkdownリンクに変換。
	markdownContent = ConvertHtmlLinksToMd(markdownContent)

	// 出力ファイルパスを生成(.html → .md、.md.md問題を回避)。
	mdPath := strings.TrimSuffix(htmlPath, ".html")
	mdPath = strings.TrimSuffix(mdPath, ".md") + ".md"
	
	// 出力ディレクトリが存在することを確認。
	mdDir := filepath.Dir(mdPath)
	if err := os.MkdirAll(mdDir, 0755); err != nil {
		return errors.Errorf("出力ディレクトリ作成エラー: %v", err)
	}
	
	// Markdownファイルを書き出し。
	if err := os.WriteFile(mdPath, []byte(markdownContent), 0644); err != nil {
		return errors.Errorf("Markdownファイル書き込みエラー: %v", err)
	}

	// ファイルが正常に作成されたか確認。
	if _, err := os.Stat(mdPath); err != nil {
		log.Printf("警告: 作成されたMarkdownファイルが見つかりません: %s", mdPath)
	} else {
		log.Printf("変換完了: %s → %s", htmlPath, mdPath)
	}
	
	return nil
}

//! 出力ディレクトリ内のHTMLファイルをリネームする。
func RenameHtmlFiles(dir string) error {
	var htmlFiles []string
	
	// HTMLファイルのパスを収集。
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// HTMLファイルのみを対象とする。
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".html") {
			htmlFiles = append(htmlFiles, path)
		}
		return nil
	})
	
	if err != nil {
		return err
	}

	// 各HTMLファイルをリネーム。
	for _, htmlPath := range htmlFiles {
		// 新しいファイル名を生成（小文字 + プレフィックス）。
		dir := filepath.Dir(htmlPath)
		filename := filepath.Base(htmlPath)
		
		// ファイル名を小文字に変換。
		lowerFilename := strings.ToLower(filename)
		newFilename := args.RenamePrefix + lowerFilename
		newPath := filepath.Join(dir, newFilename)

		// 同名ファイルが既に存在する場合はスキップ。
		if _, err := os.Stat(newPath); err == nil {
			log.Printf("リネーム先ファイルが既に存在するためスキップ: %s", newPath)
			continue
		}

		// ファイルをリネーム。
		if err := os.Rename(htmlPath, newPath); err != nil {
			log.Printf("HTMLファイルリネームエラー %s → %s: %v", htmlPath, newPath, err)
			continue // エラーが発生しても他のファイルの処理を続行。
		}

		log.Printf("リネーム完了: %s → %s", htmlPath, newPath)
	}

	return nil
}

//! Markdown内のHTMLリンクを.mdリンクに変換する。
func ConvertHtmlLinksToMd(content string) string {
	// リンクパターンをマッチする正規表現。
	// [text](path.html) または [text](path.html#anchor) の形式。
	linkPattern := regexp.MustCompile(`\[([^\]]*)\]\(([^)]*\.html)([^)]*)\)`)
	
	// HTMLリンクを.mdリンクに置換。
	result := linkPattern.ReplaceAllStringFunc(content, func(match string) string {
		// マッチした部分を解析。
		matches := linkPattern.FindStringSubmatch(match)
		if len(matches) < 4 {
			return match // 解析失敗時は元のまま。
		}
		
		linkText := matches[1]    // リンクテキスト。
		htmlPath := matches[2]    // HTMLファイルパス。
		anchor := matches[3]      // アンカー部分(#section等)。
		
		// リネーム後のHTMLファイル名を考慮したリンクに変換。
		// 相対パスの場合、プレフィックスを追加。
		if !strings.HasPrefix(htmlPath, "http") && !strings.HasPrefix(htmlPath, "/") {
			// パス区切り文字を統一 (Windows環境での%5C問題を回避)。
			htmlPath = strings.ReplaceAll(htmlPath, "\\", "/")
			
			// 相対パスの場合、ファイル名部分にプレフィックスを追加。
			dir := filepath.Dir(htmlPath)
			filename := filepath.Base(htmlPath)
			
			// .htmlを.mdに変換。
			baseFilename := strings.TrimSuffix(filename, ".html")
			baseFilename = strings.TrimSuffix(baseFilename, ".md")
			var mdPath string
			if dir == "." || dir == "" {
				mdPath = baseFilename + ".md"
			} else {
				// ディレクトリ部分を小文字に変換、ファイル名は維持。
				lowerDir := ConvertDirectoryToLowercase(dir)
				// パス区切り文字を/で統一。
				mdPath = strings.ReplaceAll(filepath.Join(lowerDir, baseFilename+".md"), "\\", "/")
			}
			
			return fmt.Sprintf("[%s](%s%s)", linkText, mdPath, anchor)
		} else {
			// 絶対パスやURLの場合は通常の変換。
			baseFilename := strings.TrimSuffix(filepath.Base(htmlPath), ".html")
			baseFilename = strings.TrimSuffix(baseFilename, ".md")
			dir := filepath.Dir(htmlPath)
			var mdPath string
			if dir == "." || dir == "" {
				mdPath = baseFilename + ".md"
			} else {
				// ディレクトリ部分を小文字に変換、ファイル名は維持。
				lowerDir := ConvertDirectoryToLowercase(dir)
				// パス区切り文字を/で統一。
				mdPath = strings.ReplaceAll(filepath.Join(lowerDir, baseFilename+".md"), "\\", "/")
			}
			return fmt.Sprintf("[%s](%s%s)", linkText, mdPath, anchor)
		}
	})
	
	return result
}

//! ディレクトリパスの各階層を小文字に変換する。ファイル名は変換しない。
func ConvertDirectoryToLowercase(dirPath string) string {
	// パス区切り文字を統一。
	dirPath = strings.ReplaceAll(dirPath, "\\", "/")
	
	// パスの各部分を分割。
	parts := strings.Split(dirPath, "/")
	
	// 各ディレクトリ名を小文字に変換。
	for i, part := range parts {
		parts[i] = strings.ToLower(part)
	}
	
	return strings.Join(parts, "/")
}

//! mdbook用のbook.tomlとSUMMARY.mdを生成する。
func GenerateMdBookFiles(outputDir string) error {
	// book.tomlを生成。
	if err := GenerateBookToml(outputDir); err != nil {
		return errors.Errorf("book.toml生成に失敗: %v", err)
	}

	// SUMMARY.mdを生成。
	if err := GenerateSummaryMd(outputDir); err != nil {
		return errors.Errorf("SUMMARY.md生成に失敗: %v", err)
	}

	return nil
}

//! book.tomlファイルを生成する。
func GenerateBookToml(outputDir string) error {
	// 出力ディレクトリ名からタイトルを生成。
	baseDirName := filepath.Base(outputDir)
	// アンダースコアをスペースに置換してタイトル化。
	title := strings.ReplaceAll(baseDirName, "_", " ")
	title = strings.ReplaceAll(title, "-", " ")
	
	// book.tomlの内容を動的生成。
	bookTomlContent := fmt.Sprintf(`[book]
title = "%s"
description = "%s"
authors = ["Generated by html2md"]
src = "%s"

[build]
build-dir = "book"
create-missing = false

[output.html]
default-theme = "navy"
preferred-dark-theme = "navy"
`, title, title, baseDirName)

	bookTomlPath := filepath.Join(outputDir, "book.toml")
	return os.WriteFile(bookTomlPath, []byte(bookTomlContent), 0644)
}

//! SUMMARY.mdファイルを生成する。
func GenerateSummaryMd(outputDir string) error {
	// ディレクトリ構造を解析（リネーム後の状態で）。
	rootEntry, err := BuildDirectoryTreeAfterRename(outputDir)
	if err != nil {
		return errors.Errorf("ディレクトリ構造解析に失敗: %v", err)
	}

	// SUMMARY.mdの内容を生成。
	var summaryBuilder strings.Builder
	summaryBuilder.WriteString("# Summary\n\n")
	
	// ルートレベルのindex.htmlまたはREADME.mdがあれば導入として追加。
	if hasIntroFile(outputDir) {
		summaryBuilder.WriteString("- [Introduction](README.md)\n\n")
	}

	// 階層構造を再帰的に出力。
	writeSummaryEntries(&summaryBuilder, rootEntry.Children, 0)

	// SUMMARY.mdファイルを書き出し。
	summaryPath := filepath.Join(outputDir, "SUMMARY.md")
	return os.WriteFile(summaryPath, []byte(summaryBuilder.String()), 0644)
}

//! ディレクトリツリーを構築する。
func BuildDirectoryTree(rootDir string) (*DirEntry, error) {
	root := &DirEntry{
		Name:     filepath.Base(rootDir),
		Path:     "",
		IsDir:    true,
		Children: []*DirEntry{},
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// ルートディレクトリ自身はスキップ。
		if path == rootDir {
			return nil
		}

		// 隠しファイルとbook.toml、SUMMARY.mdはスキップ。
		name := info.Name()
		if strings.HasPrefix(name, ".") || name == "book.toml" || name == "SUMMARY.md" {
			return nil
		}

		// プレフィックス付きのHTMLファイル（リネーム後）はスキップ。
		if strings.HasSuffix(strings.ToLower(name), ".html") && strings.HasPrefix(name, args.RenamePrefix) {
			return nil
		}

		// 相対パスを計算。
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		// パス区切り文字を/で統一。
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		// HTMLファイルの場合は.mdファイルに置き換え。
		displayPath := relPath
		if strings.HasSuffix(strings.ToLower(name), ".html") {
			// ディレクトリ部分を小文字に変換。
			dir := filepath.Dir(relPath)
			filename := filepath.Base(relPath)
			baseFilename := strings.TrimSuffix(filename, ".html")
			baseFilename = strings.TrimSuffix(baseFilename, ".md")
			
			if dir == "." || dir == "" {
				displayPath = baseFilename + ".md"
			} else {
				lowerDir := ConvertDirectoryToLowercase(dir)
				displayPath = lowerDir + "/" + baseFilename + ".md"
			}
		}

		entry := &DirEntry{
			Name:     name,
			Path:     displayPath,
			IsDir:    info.IsDir(),
			Children: []*DirEntry{},
		}

		// 親ディレクトリを見つけて追加。
		// パス区切り文字を/で統一してから検索。
		parentPath := strings.ReplaceAll(filepath.Dir(relPath), "\\", "/")
		parent := findParentEntry(root, parentPath)
		if parent != nil {
			parent.Children = append(parent.Children, entry)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 各ディレクトリの子要素をソート。
	sortDirectoryTree(root)
	return root, nil
}

//! リネーム後のディレクトリツリーを構築する。
func BuildDirectoryTreeAfterRename(rootDir string) (*DirEntry, error) {
	root := &DirEntry{
		Name:     filepath.Base(rootDir),
		Path:     "",
		IsDir:    true,
		Children: []*DirEntry{},
	}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// ルートディレクトリ自身はスキップ。
		if path == rootDir {
			return nil
		}

		// 隠しファイルとbook.toml、SUMMARY.mdはスキップ。
		name := info.Name()
		if strings.HasPrefix(name, ".") || name == "book.toml" || name == "SUMMARY.md" {
			return nil
		}

		// プレフィックス付きのHTMLファイル（リネーム後）はスキップ。
		if strings.HasSuffix(strings.ToLower(name), ".html") && strings.HasPrefix(name, args.RenamePrefix) {
			return nil
		}

		// 相対パスを計算。
		relPath, err := filepath.Rel(rootDir, path)
		if err != nil {
			return err
		}

		// パス区切り文字を/で統一。
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		// .mdファイルのみを対象とする。
		displayPath := relPath
		if strings.HasSuffix(strings.ToLower(name), ".md") {
			// .md.md問題を回避して小文字パスを生成。
			tempPath := strings.TrimSuffix(relPath, ".md")
			tempPath = strings.TrimSuffix(tempPath, ".md") + ".md"
			displayPath = strings.ToLower(tempPath)
		} else if !info.IsDir() {
			// .mdファイル以外のファイルはスキップ。
			return nil
		}

		entry := &DirEntry{
			Name:     name,
			Path:     displayPath,
			IsDir:    info.IsDir(),
			Children: []*DirEntry{},
		}

		// 親ディレクトリを見つけて追加。
		// パス区切り文字を/で統一してから検索。
		parentPath := strings.ReplaceAll(filepath.Dir(relPath), "\\", "/")
		parent := findParentEntry(root, parentPath)
		if parent != nil {
			parent.Children = append(parent.Children, entry)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// 各ディレクトリの子要素をソート。
	sortDirectoryTree(root)
	return root, nil
}

//! 指定パスの親エントリを見つける。
func findParentEntry(root *DirEntry, targetPath string) *DirEntry {
	if targetPath == "" || targetPath == "." {
		return root
	}

	// パス区切り文字を/で統一。
	targetPath = strings.ReplaceAll(targetPath, "\\", "/")
	parts := strings.Split(targetPath, "/")
	current := root

	for _, part := range parts {
		found := false
		for _, child := range current.Children {
			if child.IsDir && child.Name == part {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	return current
}

//! ディレクトリツリーをソートする。
func sortDirectoryTree(entry *DirEntry) {
	if !entry.IsDir {
		return
	}

	// 子要素をソート(ディレクトリ優先、その後アルファベット順)。
	for i := 0; i < len(entry.Children); i++ {
		for j := i + 1; j < len(entry.Children); j++ {
			a, b := entry.Children[i], entry.Children[j]
			
			// ディレクトリを優先。
			if a.IsDir && !b.IsDir {
				continue
			}
			if !a.IsDir && b.IsDir {
				entry.Children[i], entry.Children[j] = b, a
				continue
			}
			
			// 同じ種類の場合はアルファベット順。
			if a.Name > b.Name {
				entry.Children[i], entry.Children[j] = b, a
			}
		}
	}

	// 再帰的にソート。
	for _, child := range entry.Children {
		sortDirectoryTree(child)
	}
}

//! SUMMARY.mdのエントリを書き出す。
func writeSummaryEntries(builder *strings.Builder, entries []*DirEntry, depth int) {
	indent := strings.Repeat("  ", depth)

	for _, entry := range entries {
		if entry.IsDir {
			// ディレクトリの場合（リンクなし）。
			builder.WriteString(fmt.Sprintf("%s  %s\n", indent, entry.Name))
			writeSummaryEntries(builder, entry.Children, depth+1)
		} else {
			// ファイルの場合(.mdファイルのみを対象)。
			if strings.HasSuffix(strings.ToLower(entry.Name), ".md") {
				// 表示名から.mdを除去。
				displayName := strings.TrimSuffix(entry.Name, ".md")
				builder.WriteString(fmt.Sprintf("%s- [%s](%s)\n", indent, displayName, entry.Path))
			}
		}
	}
}

//! 導入ファイルの存在確認。
func hasIntroFile(dir string) bool {
	introFiles := []string{"index.html", "README.md", "readme.md"}
	for _, file := range introFiles {
		if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
			return true
		}
	}
	return false
}

//! ディレクトリ名を小文字にリネームする。Windows環境対応。
func RenameDirectoriesToLowercase(rootDir string) error {
	var directories []string
	
	// 全ディレクトリのパスを収集（深い階層から順に処理するため逆順で格納）。
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// ディレクトリのみを対象とし、ルートディレクトリは除外。
		if info.IsDir() && path != rootDir {
			directories = append(directories, path)
		}
		return nil
	})
	
	if err != nil {
		return err
	}
	
	// 深い階層から順に処理するため逆順にソート。
	for i := len(directories)/2 - 1; i >= 0; i-- {
		opp := len(directories) - 1 - i
		directories[i], directories[opp] = directories[opp], directories[i]
	}
	
	// 各ディレクトリをリネーム。
	for _, dirPath := range directories {
		if err := renameSingleDirectoryToLowercase(dirPath); err != nil {
			log.Printf("ディレクトリリネームエラー %s: %v", dirPath, err)
			continue // エラーが発生しても他のファイルの処理を続行。
		}
	}
	
	return nil
}

//! 単一ディレクトリを小文字にリネームする。
func renameSingleDirectoryToLowercase(dirPath string) error {
	dir := filepath.Dir(dirPath)
	currentName := filepath.Base(dirPath)
	lowerName := strings.ToLower(currentName)
	
	// 既に小文字の場合はスキップ。
	if currentName == lowerName {
		return nil
	}
	
	finalPath := filepath.Join(dir, lowerName)
	
	// 目的のディレクトリが既に存在する場合の処理。
	if _, err := os.Stat(finalPath); err == nil {
		log.Printf("リネーム先ディレクトリが既に存在: %s", finalPath)
		
		// 既存の小文字ディレクトリの内容を大文字ディレクトリにマージ。
		if err := mergeDirectories(finalPath, dirPath); err != nil {
			return errors.Errorf("ディレクトリマージ失敗 %s → %s: %v", finalPath, dirPath, err)
		}
		
		log.Printf("既存小文字ディレクトリをマージ完了: %s → %s", finalPath, dirPath)
		// 削除はせず、マージのみ実行。
		// 大文字ディレクトリに統合されたファイルがあるため、そのまま大文字→小文字リネームを続行。
	}
	
	// ソースディレクトリが存在することを確認。
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		log.Printf("リネーム対象ディレクトリが存在しません: %s", dirPath)
		return nil // 既に処理済みとして正常終了。
	}
	
	// Windows環境では大文字小文字の違いのみのリネームは直接できないため、
	// 一時的に別の名前にリネームしてから目的の名前にリネーム。
	tempName := currentName + "_temp_rename"
	tempPath := filepath.Join(dir, tempName)
	
	// ステップ1: 現在の名前 → 一時的な名前。
	if err := os.Rename(dirPath, tempPath); err != nil {
		return errors.Errorf("一時リネーム失敗 %s → %s: %v", dirPath, tempPath, err)
	}
	
	// ステップ2: 一時的な名前 → 小文字の名前。
	if err := os.Rename(tempPath, finalPath); err != nil {
		// 失敗した場合は元の名前に戻す。
		os.Rename(tempPath, dirPath)
		return errors.Errorf("最終リネーム失敗 %s → %s: %v", tempPath, finalPath, err)
	}
	
	log.Printf("ディレクトリリネーム完了: %s → %s", dirPath, finalPath)
	return nil
}

//! 2つのディレクトリの内容をマージする。
func mergeDirectories(srcDir, dstDir string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// ソースディレクトリ自身はスキップ。
		if path == srcDir {
			return nil
		}
		
		// 相対パスを計算。
		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		
		dstPath := filepath.Join(dstDir, relPath)
		
		if info.IsDir() {
			// ディレクトリの場合は作成。
			if err := os.MkdirAll(dstPath, info.Mode()); err != nil {
				return err
			}
		} else {
			// ファイルの場合はコピー。
			// 既に存在する場合は上書きしない（大文字ディレクトリのファイルを優先）。
			if _, err := os.Stat(dstPath); err == nil {
				log.Printf("ファイルが既に存在するためスキップ: %s", dstPath)
				return nil
			}
			
			if err := CopyFile(path, dstPath); err != nil {
				return err
			}
		}
		
		return nil
	})
}