# 使用例

## 実行ファイルの配置と動作

`secret_manager.exe`は、実行ファイルが配置されたディレクトリを基準に動作します。

```
C:\project\
├── secret_manager.exe
├── secret/
│   ├── database.ini
│   ├── database.ini.symlink.json
│   ├── api.key
│   └── api.key.symlink.json
├── app/
└── backup/
```

## 設定ファイルの例

`secret/database.ini.symlink.json`:
```json
{
  "targets": [
    {
      "path": "app/database.ini",
      "description": "Application database config"
    },
    {
      "path": "backup/db_config.ini",
      "description": "Backup database config"
    }
  ]
}
```

## 実行方法

どこからでも実行可能：
```bash
# 実行ファイルと同じディレクトリから
C:\project> secret_manager.exe

# 別のディレクトリから
C:\anywhere> C:\project\secret_manager.exe

# PATHに追加されている場合
C:\anywhere> secret_manager
```

すべての場合で、実行ファイルが存在する`C:\project`を基準にシンボリックリンクが作成されます。

## 相対パスでのシンボリックリンク

作成されるシンボリックリンクは相対パスを使用：
- `app/database.ini` → `secret/database.ini`
- `backup/db_config.ini` → `secret/database.ini`

これにより、プロジェクトディレクトリ全体を移動しても、シンボリックリンクは正しく機能します。