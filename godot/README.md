# Godotで訳文の表示を確認する

`game-l10n-updater` の表示チェック用に、Godot 4.xで動く再利用可能なアドオンと独立したサンプルをまとめています。文字数の目安ではなく、テーマのフォントとControlの矩形をレイアウト後に測ります。

検査対象にするノードは明示的に指定します。

```gdscript
node.set_meta("l10n_key", "ui.save")          # 必須
node.set_meta("l10n_check_glyphs", true)      # 任意。ノード単位のグリフ検査
```

対応するControlは `Label`、通常の `Button`、`RichTextLabel` です。指定したノードがこれ以外なら `unsupported_control` として報告し、黙って通しません。アイコンや装飾を追加するButtonの派生（`CheckBox`、`CheckButton`、`OptionButton`、`MenuButton` など）は、追加レイアウトを測定していないため `unsupported_mode` として報告します。

外部アドオン、ダウンロードした画像、ダウンロードしたフォントは不要です。サンプルはGodotの既定テーマを使います。

## 動作環境

- 対象APIレベルはGodot 4.3以上です。
- WindowsのヘッドレスモードでGodot **4.7.2.stable.mono** を使って確認しています。
- このディレクトリはモデルのダウンロード、APIキー、ネットワーク呼び出しを使いません。
- 以下のコマンドの `godot` は、インストールしたGodotの実行ファイルに置き換えてください。

## コマンド

リポジトリのルートで実行します。

### 1. 初回のインポート

新しくチェックアウトした状態では、シーンやSceneTreeのテストより先にインポートが必要です。`class_name` の登録とサンプルシーンを解決するために実行します。

```powershell
godot --headless --path godot --import
```

`godot/.godot/` ができます。このディレクトリはリポジトリの管理対象外です。

### 2. 自動テスト

```powershell
godot --headless --path godot --script res://tests/run_tests.gd
```

テストが失敗すると終了コード1を返します。確認項目は次のとおりです。

- フラットJSONとGodot CSVの読み込み、ロケール選択、引用符付きカンマ、Unicode
- 1行のLabelの溢れと、収まる文字列
- 折り返しLabelの高さ（収まる場合と不足する場合）
- 元のノードテキスト、自動翻訳モード、`TranslationServer` のロケールの復元
- 検査対象として指定した未対応Control
- レポートのスキーマ、絶対パスの出力、`res://` への書き込み拒否
- グリフ欠落ヘルパーの動作
- サンプルシーン: 意図的に切れる日本語ボタンが1件、収まる英語文字列は問題0件

テストが生成するファイルは `GAMEL10N_TEST_TMP`、`TEMP`、`TMPDIR`、`/tmp` のいずれかの下に一意な子ディレクトリを作って置き、テスト終了時に削除します。Godotのログを残す場合は `--log-file` で出力先を指定してください。

### 3. ヘッドレスで表示レポートを出す

```powershell
godot --headless --path godot --script res://tools/headless_check.gd -- `
  --report-path "$env:TEMP/l10n-demo-report.json"
```

既定では日本語と英語を検査し、統合レポートを書きます。サンプルに対して、日本語の `demo.tight_action` が1件だけ `clipped` になり、英語の文字列は通ることを期待します。結果が異なれば終了コード1を返します。

`--check-glyphs` を付けると、フォントチェーンで見つからなかったグリフもレポートに加えます。ヘッドレスの既定テーマフォントでは、システムのフォールバックで描画されるCJKでも「フォントチェーンにグリフがない」と報告されることがあります。そのため、このオプションを付けた場合、サンプルのランナーは `missing_glyph` をレイアウト判定から外します。レポートには確認用として残ります。

外部の翻訳データを指定した場合は、レポートを生成するだけで、サンプル固定の期待値は適用しません。任意の翻訳データにサンプルのキーが入っているとは限らないためです。

```powershell
godot --headless --path godot --script res://tools/headless_check.gd -- `
  --catalog "$env:TEMP/catalog_en.json" --locale en `
  --report-path "$env:TEMP/l10n-external-report.json"
```

サンプルの結果を確認したい場合は、外部の翻訳データにもサンプルのキーを含めてください。別のゲームシーンを検査する場合は、後述の手順でアドオンをそのプロジェクトへ組み込みます。

### 4. サンプルUI

```powershell
godot --path godot
```

サンプルは1000x650のControlシーンです。

- 英語 / 日本語を切り替えるボタンが2つ
- 収まるラベルと、意図的に幅が足りない翻訳ボタン
- レポートの要約とissue一覧
- 安定したノード名と `metadata/l10n_key`
- PLaMoの表記

`Run display check` は現在の翻訳データを適用し、レイアウトを待ってから指定したノードだけを検査します。レポートの要約と一覧を表示した後、表示中のテキストを元に戻します。サンプルの日本語と英語は手書きの固定データで、モデルの出力ではありません。

### 5. レポートだけを出力する

```powershell
godot --headless --path godot -- --check-only `
  --report-path "$env:TEMP/l10n-demo-report.json"
```

サンプルはレポートを書いた後に終了します。アプリへの引数は `--` の後に渡します。`--report-path` を省略した場合、レポートは `user://l10n_text_check/demo-report-<locale>.json` に書かれます。

外部のフラットJSON:

```powershell
godot --headless --path godot -- --check-only `
  --catalog "$env:TEMP/catalog_en.json" --locale en `
  --report-path "$env:TEMP/l10n-external-report.json"
```

外部のGodot CSV（`--locale` で列を選ぶ）:

```powershell
godot --headless --path godot -- --check-only `
  --catalog "$env:TEMP/translations.csv" --locale ja `
  --report-path "$env:TEMP/l10n-csv-report.json"
```

### 6. スクリーンショットを保存する

スクリーンショットは実際に描画したフレームだけを保存します。ヘッドレス実行では保存せず、その旨を表示したうえでJSONレポートは書きます。

```powershell
godot --path godot -- --check-only `
  --report-path "$env:TEMP/l10n-demo-report.json" `
  --capture-path "$env:TEMP/l10n-demo.png"
```

スクリーンショットは任意の記録です。

## レポートの形式

```json
{
  "schema_version": 1,
  "issues": [
    {
      "key": "demo.tight_action",
      "locale": "ja",
      "node_path": "Layout/Content/Columns/Left/ButtonRow/TightButton",
      "kind": "clipped",
      "message": "Button text is 190.0 px wide but only 172.0 px is available (clip_text=true)."
    }
  ],
  "checked": 12
}
```

- `schema_version` は常に `1` です。
- `node_path` は検査を開始したルートからの相対パスです。`.` はルート自身を表します。安定したパスにするため、ノード名は変更しないでください。
- `checked` はこの呼び出しで検査対象にしたノード数です。`missing_key` や `unsupported_control` を出したノードも含みます。複数言語を検査する場合は、ロケールごとに1回ずつ実行してください。
- `kind` の値:
  - `clipped` — Controlによってテキストや内容が切り取られている
  - `overflow` — テキストが測定した内容矩形の外に描画されている
  - `missing_glyph` — フォントチェーンにグリフがない（任意の検査）
  - `missing_key` — 指定したキーの翻訳データがない
  - `unsupported_control` — 指定したノードの種類を測定していない
  - `unsupported_mode` — スクロールするRichTextLabelの内容が矩形を超える場合など、安全に確認できないレイアウト
- `message` は人が読める説明です。必要に応じて測定したピクセル値を含みます。

レポートの対象は、検査したサブツリーとロケールだけです。未確認のシーンは含みません。

## 翻訳データの形式

### ターゲットJSON

1ファイルにつき1ロケールです。ロケール名は `--locale` で指定します（既定は `en`）。

```json
{
  "ui.title": "Save complete",
  "ui.action": "Continue"
}
```

値は文字列にしてください。ロケールを入れ子にしたマップはエラーになります。

### Godot CSV

先頭のヘッダーセルは `keys`（または `key`）です。続くセルがロケールです。

```csv
keys,en,ja
ui.title,Save complete,保存しました
ui.action,"Continue, now","このまま続行する"
```

引用符付きのカンマを扱えます。列は `--locale` で選びます。存在しないロケールを指定すると、利用できる列を表示してエラーになります。

## アドオンの組み込み

1. `godot/addons/l10n_text_check/` をゲームの `addons/` にコピーします。エディタプラグインの有効化は任意です。インポート後、実行時の `class_name` クラスを利用できます。
2. 検査対象にするControlだけに、安定した名前のノードで `metadata/l10n_key` を設定します。装飾用のUIや、動的に生成するだけのレポートUIは、検査対象に含めないでください。
3. 翻訳はゲームの通常のローカライズ経路で適用します。ヘルパーから翻訳データを直接適用する場合は、`L10nCatalog` と `L10nTextChecker.apply_catalog(root, catalog)` を使います。
4. 対象シーンをツリーに追加し、レイアウトが落ち着いてからチェッカーを実行します。チェッカーは `settle_frames`（既定2）だけ待ち、元のテキストと自動翻訳モードを復元します。

```gdscript
var catalog := L10nCatalog.new()
if not catalog.load_path("res://i18n/en.json", "en"):
    push_error(catalog.last_error)
    return

var checker := L10nTextChecker.new()
var report: Dictionary = await checker.check_scene(
    get_tree().current_scene,
    catalog,
    "en",
    {"settle_frames": 2, "check_glyphs": true}
)
if not checker.last_error.is_empty():
    push_error(checker.last_error)
    return

var write_error := L10nReport.write(report, "user://reports/display-en.json")
if not write_error.is_empty():
    push_error(write_error)
```

5. 生成したJSONレポートは、CLIの `check -display-report` で読み込みます。シーンとロケールごとに実行し、スクリーンショットや1シーンのレポートから他のシーンの結果を推測しないでください。

`L10nReport.write` は `res://` への書き込みを拒否します。生成したレポートが管理対象のソースに紛れ込まないようにするためです。`user://` か、リポジトリ外の絶対パスを指定してください。

## 測定する内容

- **Label:** `get_line_count()` / `get_visible_line_count()`、実効フォントの高さと行間から求めた必要な高さ、`Font.get_string_size`（折り返しなし）または利用可能な幅での `Font.get_multiline_string_size`（折り返しあり）を使います。`clip_text` と `text_overrun_behavior` で `clipped` と `overflow` を分けます。
- **Button:** 実効テーマフォント、styleboxの内容マージン、アイコン幅と `h_separation` を求め、残りの内容矩形に対してテキストの幅と高さを測ります。`clip_text` で `clipped` と `overflow` を分けます。
- **RichTextLabel:** 内容の幅と高さを内容矩形と比べます。スクロールが有効で内容が矩形を超える場合は、誤って通さず `unsupported_mode` にします。
- **グリフ欠落:** 実効テーマフォントと `ThemeDB.fallback_font` に対して `Font.has_char` を使います。システムのフォールバックフォントは確実に列挙できないため、全体の `check_glyphs` かノード単位の `l10n_check_glyphs` で明示した場合だけ検査します。レポートは「フォントチェーンにグリフがない」と書き、システムのフォールバックが描画できなかったと断定しません。
- **復元:** 各検査の後、元のノードテキスト、`auto_translate_mode`、`TranslationServer` のロケールを戻します。サンプルで言語を切り替える操作は表示ロケールを意図的に変えますが、チェッカー自身は別のロケールを残しません。

## 確認済みの範囲

自動テストとヘッドレスランナーで確認している内容です。

- フラットJSONとGodot CSVの読み込み、ロケール切り替え、Unicode、引用符付きカンマ
- Labelの溢れと、収まる文字列
- 折り返しLabelで高さが足りる場合と足りない場合
- 意図的に狭い翻訳ボタンによるButtonの切れ
- RichTextLabelの非スクロール時の溢れと、スクロール時の `unsupported_mode`
- テキストとロケールの復元
- 明示した未対応Control（`unsupported_control`）
- レポートのスキーマと、絶対パス / `user://` への書き込み
- 未収録の私用領域コードポイントに対するグリフ検査ヘルパー
- サンプルシーンで、意図的に切れる日本語ボタンが1件、収まる英語文字列は問題なし

確認していない範囲は次のとおりです。

- プロジェクト内の全シーンと全Control — 検査するのは指定したサブツリーとロケールだけです。
- `l10n_key` がないノード
- カスタム描画のテキスト、`LineEdit`、`TextEdit`、`Label3D`、3D世界、このControlの対象外で描画されるゲームHUD
- RichTextLabelのグリフ網羅、BBCodeを考慮した区間ごとのフォント選択、スクロール内容の完全な検証（スクロール時の溢れは `unsupported_mode`）
- `expand_icon` を使うButton（`unsupported_mode`）
- `autowrap_mode` を使うButton（`unsupported_mode`）
- 追加のアイコンや装飾を描くButtonの派生（`CheckBox`、`CheckButton`、`OptionButton`、`MenuButton` など）— 誤って通さず `unsupported_mode` として報告します。
- 検査後に読み込まれるフォントや翻訳、アニメーション、シーン遷移、検査の前後にゲーム処理で変わるテキスト
- 非表示の検査対象ノード — 描画されないため `unsupported_mode` として報告します。
- グリフ欠落時のシステムフォールバック描画

## PLaMoの表記とライセンス

`demo/translations.csv` とサンプルのJSONにある文字列は手書きの固定データです。PLaMoモデルの出力ではないため、モデル出力として引用しないでください。

実際の翻訳データには、PLaMoモデルが生成した訳文が含まれる場合があります。PLaMoモデルの条件と、親プロジェクトの文書（リポジトリの `THIRD_PARTY_NOTICES.md` と `README.md`）からリンクされている表記・条件に従ってください。このサンプルがそれらを生成したように見せないでください。このディレクトリはネットワーク呼び出しもモデル呼び出しもしません。

アドオンとサンプルはリポジトリのMITライセンスで提供されます。外部からダウンロードした画像やフォントは含みません。
