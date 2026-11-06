# Godot 4.x 表示チェックとの連携

> このリポジトリの `godot/` に、再利用できるアドオンと独立したサンプルを含めています。
> Windows / Godot 4.7.2 で、実モデルの翻訳 CSV を読み込み、文字切れを検出して
> CLI に取り込む流れを確認しました。対応する Control と制限は
> [../godot/README.md](../godot/README.md) を参照してください。

## 役割分担

| 担当 | 内容 |
| --- | --- |
| game-l10n-updater（このリポジトリ） | 差分計画、API 呼び出し、手動翻訳保護、プレースホルダー検証、トランザクション、Godot CSV/JSON 入出力、表示レポートのマージ |
| `godot/` アドオン | 選択したシーン内の対象ノードに翻訳を適用し、Control のフォント・寸法を測定して表示レポートを生成 |
| local-jp-translate | ループバック `POST /translate` によるローカル翻訳 |
| 人間 | 意味レビュー、承認、ロック、許容する折り返しの判断 |

## 想定パイプライン

1. `gamel10n export -format csv -out dist/game.csv` で Godot 翻訳 CSV を出す。
2. Godot 側で CSV を翻訳リソースとして import する。
3. Godot 側で対象シーンを実行し、翻訳後の Control の表示を測る。
4. 溢れ・切れ・はみ出しを `{schema_version:1, issues:[...], checked:N}` に出す。
5. `gamel10n check --display-report reports/godot-display.json --report reports/check.json`
   でキー/ロケールと結び付ける。
6. 表示 issue と `max_length` 警告を見て、人手でターゲットを直す。
7. 必要なら `gamel10n approve` で承認し、`sync` で次の差分へ進む。

サンプルでは `godot --headless --path godot --script res://tools/headless_check.gd -- --catalog /absolute/translated.csv --locale en --report-path /absolute/display.json` で手順 3〜4 を実行できます。

## 表示レポート契約

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

- `schema_version`: `1` 固定。違う値は `check` が設定エラーにします。
- `issues[].key`: 原文データのキー。原文データにないキーは
  `unknown_key: true` として残し、`display_report.unknown_keys` にも出します。
- `issues[].locale`: 表示したロケール。source/target の設定にないロケールは
  `unknown_locale: true` として残し、`display_report.unknown_locales` にも出します。
  設定済みのターゲットロケールだけを検査対象にできます（`check -locale`）。
- `issues[].node_path`: Godot のノードパスなどの位置情報。CLI は解釈せず保持します。
- `issues[].kind`: `clipped` / `overflow` / `missing_glyph` / `missing_key` /
  `unsupported_control` / `unsupported_mode` など。CLI は列挙を強制しませんが、
  `clipped` / `overflow` だけを確定した表示エラーとして扱います。
- `issues[].message`: 任意の説明。
- `checked`: 検査したノード数などの自己申告（0 以上）。

ファイルは `check --display-report PATH` で読みます。未知フィールドは拒否します。
`checked` は検査範囲の証明ではなく、スクリーンショットの有無も全シーン実行の
証明ではありません。

## 表示 issue と `max_length` の違い

| 種別 | 由来 | 意味 | 終了コードへの影響 |
| --- | --- | --- | --- |
| `max_length` | `check` のルーン数 | 文字数の予備警告。描画幅の証明ではない | 警告のみ。0 |
| 表示 issue (`clipped` / `overflow`) | Godot レポート | 実際に描画した結果の溢れ/切れ | 構造エラーがなければ 4 |
| 表示 issue (`missing_glyph` / `missing_key` / `unsupported_*`) | Godot レポート | フォント未確認、キー欠落、未対応 Control など。確定した溢れではない | 警告のみ。0 |

`max_length` を超えていても実際に収まることはあります。逆に、ルーン数が短くても
フォントや解像度で溢れることがあります。最終判断は実表示と人間レビューです。
`missing_glyph` はシステムフォールバックで描画される場合があるため、Godot 側も
助言的と明記しています。

## 表示レポートの生成側で必要なこと

付属アドオン以外でレポートを生成する場合も、次の情報を記録してください。

- ロケールごとに翻訳リソースを適用し、表示対象のシーン/Control を実際に
  レイアウトした後に測る。
- `Control` の表示テキスト、最小サイズ、実際の矩形、クリップ状態などから
  `kind` と `message` を作る。
- 原文データの安定キーを `key` に、適用ロケールを `locale` に、
  再現可能なノード位置を `node_path` に入れる。
- `checked` は「何を数えたか」を実装側で定義する（例: 検査した Control 数）。
- 未訪問シーン、非表示シーン、動的生成ノードを「検査済み」に数えない。
- スクリーンショットを保存する場合も、それは補助証拠であり、`checked` の
  意味を拡張しない。

## 実行例

```sh
# 表示レポートをマージし、JSON レポートも保存する
gamel10n check -config gamel10n.json \
  --display-report reports/godot-display.json \
  --report reports/check.json

# 構造エラーがなければ exit 0、表示エラーだけなら exit 4
echo $?
```

CI や実ゲームでの受入では、隔離した Godot サンプルプロジェクトを使ってください。
このリポジトリの `examples/sample/` は JSON/CSV のサンプルであり、Godot プロジェクト
ではありません。
