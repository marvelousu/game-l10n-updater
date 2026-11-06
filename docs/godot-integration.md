# Godot 4.x の表示チェック

`godot/` に、再利用できるアドオンと独立したサンプルを含めています。Windows / Godot 4.7.2 で、実モデルの翻訳CSVを読み込み、文字切れを検出してCLIに取り込む流れを確認しました。対応するControlと制限は [../godot/README.md](../godot/README.md) を参照してください。

## 役割

| 担当 | 内容 |
| --- | --- |
| game-l10n-updater（このリポジトリ） | 差分計画、API呼び出し、手動訳の保護、プレースホルダー検査、保存時のロールバック、Godot CSV/JSONの入出力、表示レポートのマージ |
| `godot/` アドオン | 指定したシーン内の対象ノードに翻訳を適用し、Controlのフォントと寸法を測って表示レポートを生成 |
| local-jp-translate | ループバック `POST /translate` によるローカル翻訳 |
| 人 | 意味の確認、承認、固定、許容する折り返しの判断 |

## 処理の順序

1. `gamel10n export -format csv -out dist/game.csv` でGodot翻訳CSVを出力する。
2. Godot側でCSVを翻訳リソースとして読み込む。
3. 対象シーンを実行し、翻訳後のControlの表示を測る。
4. 溢れ・切れを `{"schema_version":1,"issues":[...],"checked":N}` に書き出す。
5. `gamel10n check -display-report reports/godot-display.json -report reports/check.json` でキーとロケールに対応付ける。
6. 表示issueと `max_length` 警告を確認し、`targets/<locale>.json` を手動で修正する。
7. 必要なら `gamel10n approve` で承認し、次の `sync` に進む。

サンプルでは `godot --headless --path godot --script res://tools/headless_check.gd -- --catalog /absolute/translated.csv --locale en --report-path /absolute/display.json` で手順3〜4を実行できます。

## 表示レポートの形式

```json
{
  "schema_version": 1,
  "issues": [
    {
      "key": "ui.greet",
      "locale": "en",
      "node_path": "MainMenu/GreetingLabel",
      "kind": "overflow",
      "message": "text exceeds its Control rect"
    }
  ],
  "checked": 42
}
```

- `schema_version`: `1` 固定です。違う値は `check` がエラーにします。
- `issues[].key`: 原文データのキーです。原文データにないキーは `unknown_key: true` として残し、`display_report.unknown_keys` にも出します。
- `issues[].locale`: 表示したロケールです。source/targetの設定にないロケールは `unknown_locale: true` として残し、`display_report.unknown_locales` にも出します。`check -locale` で設定済みのターゲットロケールだけに絞り込めます。
- `issues[].node_path`: Godotのノードパスなどの位置情報です。CLIは解釈せずに保持します。
- `issues[].kind`: `clipped` / `overflow` / `missing_glyph` / `missing_key` / `unsupported_control` / `unsupported_mode` などです。CLIは列挙を強制しませんが、`clipped` / `overflow` だけを確定した表示エラーとして扱います。
- `issues[].message`: 任意の説明です。
- `checked`: レポートを生成した側が数えた検査件数です（0以上）。

ファイルは `check -display-report PATH` で読みます。未知フィールドは拒否します。検査対象は、アドオンで指定したシーン内のUI部品です。

## 表示の検査と文字数の検査

| 種別 | 由来 | 意味 | 終了コードへの影響 |
| --- | --- | --- | --- |
| `max_length` | `check` のルーン数 | 文字数の予備警告。描画幅の確認ではない | 警告のみ。0 |
| 表示issue (`clipped` / `overflow`) | Godotレポート | 実際に描画した結果の溢れ・切れ | 構造エラーがなければ4 |
| 表示issue (`missing_glyph` / `missing_key` / `unsupported_*`) | Godotレポート | フォント未確認、キー欠落、未対応Controlなど。確定した溢れではない | 警告のみ。0 |

`max_length` を超えていても実際に収まることはあります。逆に、ルーン数が短くてもフォントや解像度で溢れることがあります。文字数の警告と併せて、実際の画面でも確認してください。`missing_glyph` はシステムのフォールバックで描画される場合があるため、Godot側も助言的な情報として扱っています。

## レポートを作る側で必要なこと

付属アドオン以外でレポートを作る場合も、次の情報を記録してください。

- ロケールごとに翻訳リソースを適用し、表示対象のシーン/Controlを実際にレイアウトした後に測る。
- `Control` の表示テキスト、最小サイズ、実際の矩形、クリップ状態などから `kind` と `message` を作る。
- 原文データの安定キーを `key` に、適用ロケールを `locale` に、再現可能なノード位置を `node_path` に入れる。
- `checked` は「何を数えたか」を実装側で定義する（例: 検査したControl数）。
- 未訪問のシーン、非表示のシーン、動的に生成したノードを「検査済み」に数えない。

## 実行例

```sh
# 表示レポートをマージし、JSONレポートも保存する
gamel10n check -config gamel10n.json \
  -display-report reports/godot-display.json \
  -report reports/check.json

# 構造エラーがなければexit 0、表示エラーだけならexit 4
echo $?
```

Godotプロジェクトのサンプルは `godot/` にあります。自分のゲームへ導入する手順は [アドオンの組み込み](../godot/README.md#アドオンの組み込み) を参照してください。
