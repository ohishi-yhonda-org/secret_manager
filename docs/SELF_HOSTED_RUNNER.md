# Self-Hosted Runner セットアップガイド

このプロジェクトはWindows self-hostedランナーを使用してGitHub Actionsを実行します。

## セットアップ手順

### 1. GitHubリポジトリでランナーを追加

1. GitHubリポジトリの Settings > Actions > Runners に移動
2. "New self-hosted runner" をクリック
3. Operating System: Windows を選択
4. Architecture: x64 を選択

### 2. ランナーのダウンロードと設定

PowerShellを管理者として開き、以下のコマンドを実行：

```powershell
# ランナー用のディレクトリを作成
mkdir C:\actions-runner
cd C:\actions-runner

# GitHubの指示に従ってランナーをダウンロード
# 例：
Invoke-WebRequest -Uri https://github.com/actions/runner/releases/download/v2.311.0/actions-runner-win-x64-2.311.0.zip -OutFile actions-runner-win-x64-2.311.0.zip

# 解凍
Expand-Archive -Path actions-runner-win-x64-2.311.0.zip -DestinationPath .

# 設定（GitHubから提供されるトークンを使用）
./config.cmd --url https://github.com/YOUR_USERNAME/YOUR_REPO --token YOUR_TOKEN
```

### 3. 必要なソフトウェアのインストール

Self-hostedランナーには以下のソフトウェアが必要です：

```powershell
# Chocolateyのインストール（まだの場合）
Set-ExecutionPolicy Bypass -Scope Process -Force
[System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))

# Goのインストール
choco install golang -y

# Gitのインストール
choco install git -y

# Cコンパイラのインストール（race detectorを使用する場合）
# オプション1: MinGW-w64
choco install mingw -y

# オプション2: Visual Studio Build Tools（より大きいが推奨）
choco install visualstudio2022buildtools -y
choco install visualstudio2022-workload-vctools -y
```

#### race detectorを使用しない場合

Cコンパイラのインストールをスキップして、`test-simple.yml`ワークフローを使用してください。

### 4. ランナーをサービスとして実行

```powershell
# サービスとしてインストール
./svc.sh install

# サービスを開始
./svc.sh start

# サービスの状態を確認
./svc.sh status
```

### 5. 環境変数の設定

システム環境変数に以下を追加：

- `GOPATH`: C:\Users\%USERNAME%\go
- `PATH`に追加: C:\Program Files\Go\bin

## トラブルシューティング

### ランナーが見つからない

- GitHubのActions設定でランナーがオンラインになっているか確認
- Windowsファイアウォールでブロックされていないか確認
- サービスが実行されているか確認: `Get-Service "actions.runner.*"`

### ビルドエラー

- Goが正しくインストールされているか確認: `go version`
- 環境変数が正しく設定されているか確認
- ランナーサービスを再起動: `./svc.sh stop` → `./svc.sh start`

## セキュリティ注意事項

- Self-hostedランナーはプライベートリポジトリでのみ使用することを推奨
- 定期的にランナーソフトウェアを更新
- 最小権限の原則に従ってランナーを設定