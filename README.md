# Secret Manager - Symlink Creator

Goで実装されたシンボリックリンク管理システムです。実行ファイルと同じディレクトリ内の`secret`を含む名前のフォルダを再帰的に検索し、`.symlink.json`設定ファイルに基づいて自動的にシンボリックリンクを作成します。

## 使い方

1. `secret`を含む名前のフォルダ（例：`secret`、`my_secrets`、`secret_config`など）にファイルを配置
2. 同じフォルダに`ファイル名.symlink.json`を作成
3. `secret_manager.exe`を実行（任意のディレクトリから実行可能）

## 設定ファイル形式

```json
{
  "targets": [
    {
      "path": "../link1/example.txt",
      "description": "Link to link1 directory"
    },
    {
      "path": "../link2/secret_example.txt", 
      "description": "Link to link2 directory with different name"
    }
  ]
}
```

## 注意事項

### シンボリックリンク作成の権限
Windowsでシンボリックリンクを作成するには管理者権限が必要です：
- 管理者権限でコマンドプロンプトを開く
- または開発者モードを有効にする（Windows 10/11）

### 動作仕様
- 実行ファイルと同じディレクトリ内で、名前に`secret`を含むすべてのフォルダを再帰的に検索します
- 各フォルダ内の`.symlink.json`ファイルを処理します
- どのディレクトリからでも実行可能（実行ファイルの場所を基準に動作）

### ディレクトリの事前作成
ターゲットディレクトリは事前に作成しておく必要があります。存在しない場合はエラーメッセージが表示され、そのターゲットはスキップされます。

### 既存ファイルの処理
ターゲットパスに既にファイルやシンボリックリンクが存在する場合、自動的に削除して新しいシンボリックリンクを作成します。

## ビルド方法

### ローカルビルド
```bash
go build -o secret_manager.exe main.go
```

### クロスプラットフォームビルド
```bash
# Linux (64-bit)
GOOS=linux GOARCH=amd64 go build -o secret_manager-linux-amd64 main.go

# macOS (Intel)
GOOS=darwin GOARCH=amd64 go build -o secret_manager-darwin-amd64 main.go

# macOS (Apple Silicon)
GOOS=darwin GOARCH=arm64 go build -o secret_manager-darwin-arm64 main.go

# Windows (64-bit)
GOOS=windows GOARCH=amd64 go build -o secret_manager-windows-amd64.exe main.go
```

## リリース

GitHubでタグをプッシュすると、自動的に各プラットフォーム用のバイナリがビルドされ、リリースページに公開されます：

```bash
git tag v1.0.0
git push origin v1.0.0
```

## GitHub Actions

このプロジェクトは以下のGitHub Actionsワークフローを使用しています：

- **test.yml**: プッシュ/PR時に自動テストを実行（Cコンパイラがあれば自動的にrace detector有効、カバレッジ95%以上を要求）
- **release.yml**: タグプッシュ時に自動的にマルチプラットフォームビルドを実行し、リリースを作成

### Self-Hosted Runner

このプロジェクトはWindows self-hostedランナーで実行されます。セットアップ方法については[docs/SELF_HOSTED_RUNNER.md](docs/SELF_HOSTED_RUNNER.md)を参照してください。

**注意**: test.ymlは自動的にCコンパイラの有無を検出し、利用可能な場合のみrace detectorを使用します。