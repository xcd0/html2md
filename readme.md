# html2md

HTMLファイルを含むディレクトリ階層をMarkdown形式に変換し、mdbook対応も可能なGoツール。
chmファイルなどhtmlになっているものをmarkdownに変換し、mdbookにしたかったため作成。

引数に与えたディレクトリにあるファイルのうち拡張子がhtmlのものをmarkdownに変換する。

ほぼClaude4製。以下の説明も。


## 機能

- **HTML→Markdown変換**: 階層構造を保持してHTMLファイルを`.md`に変換
- **リンク修正**: HTML内の相対リンクを自動的にMarkdownリンクに変換  
- **HTMLファイルリネーム**: 元のHTMLファイルにプレフィックスを付与
- **mdbook対応**: `book.toml`と`SUMMARY.md`を生成

## 使用方法

```bash
# 基本的なHTML→Markdown変換
./html2md ./source_directory

# カスタムサフィックス指定
./html2md ./source_directory -s "_output"

# HTMLファイルのプレフィックス変更
./html2md ./source_directory --rename-prefix "original_"

# mdbook用ファイル生成(この時変換処理は行わない。)
./html2md ./source_directory -b
```

## オプション

- `-s, --suffix`: 出力ディレクトリのサフィックス (デフォルト: `_converted`)
- `--rename-prefix`: 元のHTMLファイル名に付与するプレフィックス (デフォルト: `_`)
- `-b, --mdbook`: mdbook用ファイル生成モード

## 出力仕様

### 通常モード
```
input_dir/
├── page.html
└── subdir/
    └── file.html

↓ 変換後

input_dir_converted/
├── _page.html      # リネーム後の元HTMLファイル
├── page.md         # 変換されたMarkdownファイル
└── subdir/
    ├── _file.html  # リネーム後の元HTMLファイル
    └── file.md     # 変換されたMarkdownファイル
```

### mdbookモード (`-b`使用時)
- HTML→Markdown変換は実行しない
- `book.toml` (mdbook設定ファイル) を生成
- `SUMMARY.md` (階層構造の目次ファイル) を生成


