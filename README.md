# game-l10n-updater

ゲームの文章を翻訳するCLIです。原文を追加・変更した箇所だけをローカルの翻訳APIに送り、`targets/<locale>.json` と状態ファイルを更新します。手動で修正した訳文は保持し、原文も変わった箇所は `needs_review` として報告します。

翻訳APIは別プロジェクトの [local-jp-translate](https://github.com/marvelousu/local-jp-translate) を使います。Godot向けに、訳文がボタンやラベルに収まるか確認するアドオンとサンプルも含みます。

## ダウンロードと展開

[GitHub Releases](https://github.com/marvelousu/game-l10n-updater/releases/latest) からOSに合うファイルを取得して展開します。

| OS / CPU | ファイル末尾 |
| --- | --- |
| Windows amd64 | `windows_amd64.zip` |
| Linux amd64 | `linux_amd64.tar.gz` |
| macOS arm64 | `darwin_arm64.tar.gz` |

Windowsのzipを展開すると、中のフォルダに `gamel10n.exe`、`examples/`、`docs/` などが入っています。以下のWindows例は、そのフォルダで実行します。PATHの設定、Go、Pythonは不要です。

Linux / macOSでは実行ファイル名が `gamel10n` です。コマンド例の `.\gamel10n.exe` を `./gamel10n` に置き換えてください。

## 翻訳APIなしで試す

`examples/quickstart/` には、日本語4件と英訳、状態ファイルを用意しています。APIやモデルの準備をせずに、翻訳対象の確認・プレースホルダーの検査・CSV出力を試せます。

```powershell
.\gamel10n.exe plan -config examples\quickstart\gamel10n.json
.\gamel10n.exe check -config examples\quickstart\gamel10n.json -report quickstart-check.json
.\gamel10n.exe export -config examples\quickstart\gamel10n.json -format csv -out quickstart.csv
Get-Content .\quickstart.csv -Encoding utf8
```

`plan` は翻訳の要否を確認するコマンドです。このサンプルは訳文を記載済みなので、`api_requests=0` になります。`check` の結果は次のとおりです。

```text
check: checked=4 errors=0 warnings=0 structural=0 display=0
```

`quickstart.csv` には次の内容が出力されます。原文と訳文の両方に `{name}` が残っていることも確認できます。

```csv
keys,ja,en
menu.new_game,はじめから,New Game
menu.quit,終了,Quit
ui.greet,こんにちは、{name}さん！,"Hello, {name}!"
ui.potion,ポーションを1個手に入れた。,You got a potion.
```

Linux / macOSでは次のコマンドで試せます。

```sh
./gamel10n plan -config examples/quickstart/gamel10n.json
./gamel10n check -config examples/quickstart/gamel10n.json -report quickstart-check.json
./gamel10n export -config examples/quickstart/gamel10n.json -format csv -out quickstart.csv
cat quickstart.csv
```

## 実際に翻訳する

[local-jp-translateのREADME](https://github.com/marvelousu/local-jp-translate#readme) の手順で翻訳APIを起動します。既定の接続先は `http://127.0.0.1:8787` です。このCLIはループバック接続だけを使います。

別のプロジェクトを作ります。

```powershell
.\gamel10n.exe init -dir my-game-l10n -source-lang ja -languages en
```

`my-game-l10n/` に次のファイルができます。`source/catalog.json` は空の原文データです。

```text
gamel10n.json
source/catalog.json
targets/en.json
state/gamel10n-state.json
```

`my-game-l10n/source/catalog.json` を次の内容に置き換えます。

```json
{
  "schema_version": 1,
  "entries": [
    {
      "key": "ui.greet",
      "text": "こんにちは、{name}さん！",
      "context": "title screen greeting"
    },
    {
      "key": "ui.potion",
      "text": "ポーションを1個手に入れた。",
      "context": "item pickup message"
    }
  ]
}
```

`plan` で翻訳対象を確認します。`new=2` は2件が未翻訳で、2回のAPI呼び出しが必要という意味です。

```powershell
.\gamel10n.exe plan -config my-game-l10n\gamel10n.json
```

```text
plan: new=2 changed=0 unchanged=0 deleted=0 needs_review=0 api_requests=2
  new          en       ui.greet (target is missing)
  new          en       ui.potion (target is missing)
```

`sync` で翻訳します。

```powershell
.\gamel10n.exe sync -config my-game-l10n\gamel10n.json
```

```text
sync success: api_calls=2 wrote_files=true new=2 changed=0 unchanged=0 needs_review=0 deleted=0 pruned=0
  translated en       ui.greet = Hello, {name}!
  translated en       ui.potion = I got one potion.
```

上の訳文は PLaMo 3 Translate 0.8B で実行した結果です。

このとき変わるのは次の2ファイルです。`gamel10n.json` と `source/catalog.json` は変わりません。

- `my-game-l10n/targets/en.json`
- `my-game-l10n/state/gamel10n-state.json`

`check` と `export` は同じプロジェクトで実行できます。

```powershell
.\gamel10n.exe check -config my-game-l10n\gamel10n.json
.\gamel10n.exe export -config my-game-l10n\gamel10n.json -format csv -out my-game.csv
```

`sync` が成功した時点でプレースホルダーの検査も通っています。`max_length` を設定していなければ、`check` は次の要約で終了します。

```text
check: checked=2 errors=0 warnings=0 structural=0 display=0
```

`my-game.csv` には `keys,ja,en` のヘッダーと2件の行が出力されます。英語の列はAPIが返した訳文です。

## 原文の変更と再翻訳

| 変更 | 次の `sync` |
| --- | --- |
| 原文にキーを追加 | 追加したキーを翻訳 |
| 自動訳のキーで `text` を変更 | そのキーを再翻訳 |
| 手動修正・承認・固定した訳がある | 訳文を保持。原文も変わっていれば `needs_review` と報告 |
| 変更なし | APIを呼ばず、翻訳データと状態も変えない |
| 原文からキーを削除 | 既定では報告のみ。`-prune-deleted` を明示すると削除 |

プレースホルダーは `{name}`、`%d`、対応するHTML / BBCodeタグの名前と個数を検査します。壊れた訳は適用しません。意味や口調がゲームに合うかは、訳文を読んで確認してください。

たとえば、`my-game-l10n/source/catalog.json` の「ポーションを1個手に入れた。」を「ポーションを2個手に入れた。」に変更すると、次の `sync` は `ui.potion` だけを再翻訳します。実行時の訳文は `I got two potions.` になりました。

```text
plan: new=0 changed=1 unchanged=1 deleted=0 needs_review=0 api_requests=1
  unchanged    en       ui.greet (source and target unchanged)
  changed      en       ui.potion (source text changed)
```

変更判定の対象は `text` です。`context` や `max_length` だけを変えても再翻訳しません。

## 訳文の修正と承認

`targets/en.json` を直接編集すると、次の `sync` では手動で修正した訳として保持されます。原文も変わった場合は訳文を残したまま `needs_review` と報告します。内容を確認して修正した後、キーを指定して承認します。`-lock` で訳文を固定でき、`unlock` で固定を解除します。

```powershell
.\gamel10n.exe approve -config my-game-l10n\gamel10n.json -locale en -key ui.greet
.\gamel10n.exe approve -config my-game-l10n\gamel10n.json -locale en -key ui.greet -lock
.\gamel10n.exe unlock -config my-game-l10n\gamel10n.json -locale en -key ui.greet
```

CSVで確認・修正したい場合は `export` と `import-csv` も使えます。既存データへの取り込み、承認・固定、保存時の保護は [操作の詳細](docs/usage.md) にまとめています。

## Godotで表示を確認する

![英訳を読み込んだGodotサンプルで、ボタンの文字切れを検出した画面](docs/images/godot-display-check.png)

PLaMoの英訳と手動で修正した訳を読み込んだ実行画面です。右側の狭いボタンは、意図的に文字が切れるようにしてあります。

付属の `godot/` をGodot 4.xで開くと、サンプルを実行できます。アドオンは、翻訳キーを指定した `Label`、通常の `Button`、`RichTextLabel` を、実際のフォントと表示領域で測ります。

```powershell
.\gamel10n.exe check -config my-game-l10n\gamel10n.json -display-report reports\godot-display.json
```

Godotの表示レポートを取り込むと、文字切れのあるキーを翻訳ファイルと対応付けて確認できます。導入とレポートの作り方は [Godot連携](docs/godot-integration.md) と [アドオンの使い方](godot/README.md) を参照してください。

## 対応範囲

- APIへの接続はループバック限定です。
- プレースホルダー検査は対応する基本構文が対象です。ICU MessageFormatやすべての書式、タグの入れ子関係を検査するものではありません。
- Godotの検査対象は、指定したシーン内の対応するUI部品です。ゲーム全体を自動巡回しません。
- Windowsで実モデルの翻訳とGodot 4.7.2の表示を確認しています。Linux / macOSの実推論・実描画は未確認です。
- 保存中に検知したエラーでは元に戻します。電源断や強制終了に対する複数ファイルの同時更新は保証しません。

終了コードは、成功 `0`、実行エラー `1`、使い方の誤り `2`、構造エラー `3`、表示エラー `4` です。

## ソースからビルドする

Go 1.23以上が必要です。依存はGo標準ライブラリのみです。リリース版を試すだけならGoは不要です。

```sh
go build -trimpath -o bin/gamel10n ./cmd/gamel10n
go test ./...
go vet ./...
```

`go test ./...` は翻訳APIやGodotの起動なしで実行できます。

Windowsでは出力名を `bin/gamel10n.exe` にします。モデルなしで試せる原文・訳文の例は `examples/quickstart/` と `examples/sample/` にあります。

配布物はWindowsで `scripts/package.ps1`、Linux / macOSで `scripts/package.sh` を使って生成できます。

## ライセンス

ソースコードは [MIT](LICENSE) です。翻訳には PLaMo 3 Translate を利用しています（Built with PLaMo）。モデル重みは同梱していません。PLaMoモデルとその出力には PLaMo Community License が適用され、個人・趣味・学術などの非商用利用は無償で登録不要です。商用利用には事前登録と売上規模の条件があります。

詳細は [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) を確認してください。
