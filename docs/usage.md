# 手動修正・CSV取り込み・保存

## 手動翻訳・承認・固定

状態ファイルは、最後に生成した翻訳データのハッシュと保護フラグを持ちます。

| フラグ | 意味 |
| --- | --- |
| `manual` | 手動で書いた、または直した翻訳。上書きしない |
| `approved` | 承認済み。上書きしない |
| `locked` | 明示的に固定。上書きしない |
| `needs_review` | 原文が変わったが保護対象なので保持した。確認待ち |

- 初回実行で状態にない翻訳データが存在する場合、その値は手動翻訳として
  `manual` にします。初回の手動訳を機械翻訳で上書きしません。
- 状態の `target_hash` と現在の翻訳データが違えば「手動編集」とみなし、API を
  呼ばずに保持します。
- 原文が変わった手動 / 承認 / 固定済みの翻訳データは、値を保持して
  `needs_review: true` にします。原文ハッシュは古いまま残すので、`plan` は
  `needs_review` を出し続けます。
- レビュー後、翻訳データを直して次のように承認します。

```sh
gamel10n approve -config gamel10n.json -locale en -key ui.greet
gamel10n approve -config gamel10n.json -locale en -key ui.greet -lock
gamel10n unlock  -config gamel10n.json -locale en -key ui.greet
```

削除されたキーは既定でレポートのみです。翻訳データも状態も保持します。
`sync -prune-deleted` を明示したときだけ削除します。

## CSVで確認・取り込みする

Godot 翻訳 CSV は `export` で出し、`import-csv` で取り込めます。出力した CSV を
表計算ソフトで確認し、翻訳列を直してから取り込みます。

```sh
# レビュー用に出す
gamel10n export -config gamel10n.json -format csv -out review.csv

# 新規プロジェクトへ取り込む
gamel10n import-csv -csv review.csv -source-language ja \
  -out source/catalog.json -targets-dir targets

# 既存の出力を置き換える
gamel10n import-csv -csv review.csv -source-language ja \
  -out source/catalog.json -targets-dir targets -force
```

- 先頭列は `keys`（`key` も可。大文字小文字可）。`-source-language` の列を原文にし、
  それ以外の列を翻訳先ロケールとして扱います。
- 空の翻訳セルは「未翻訳」として省略します。既存の翻訳データへマージするときは、
  空にしたキーの既存値が残ります。
- 既存の出力ファイルがある場合は `-force` が必要です。`-force` は出力ファイルを
  置き換えます。状態の `manual` / `approved` / `locked` は `sync` に対する保護で、
  `import-csv` には適用されません。手動訳を残したいセルは空にするか、取り込み後に
  差分を確認してください。
- import は状態ファイルを作りません。取り込んだ翻訳データは次の `sync` で
  手動訳として保持されます。
- CSV にない原文キーは出力から消え、`context` / `max_length` も復元できません。
  既存プロジェクトでこれらを保ちたい場合は、CSV を取り込まず
  `targets/<locale>.json` を直接編集するか、取り込み後に原文データへ追記してください。

## プレースホルダーとタグの規則

原文と翻訳の**完全な多重集合**を比較します。順序は無視しますが、数と種類は一致
しなければなりません。

| 種類 | 例 | 規則 |
| --- | --- | --- |
| 波括弧 | `{name}`, `{0}` | 1 個ずつ数える。`{{` と `}}` はエスケープで、プレースホルダーにしない |
| パーセント | `%s`, `%d`, `%02d`, `%.2f`, `%1$s` | 変換指定全体を 1 トークンとして完全一致。`%%` はリテラル |
| HTML | `<b>`, `</b>`, `<br/>` | 基本タグ名の開閉を数える。大文字小文字は同一視、属性は無視。`<br/>` と `<br>` は同じ。未知の `<Player>` などはリテラル |
| Godot BBCode | `[b]`, `[/b]`, `[color=red]` | 基本タグ名の開閉を数える。大文字小文字は同一視、`=値` は無視。未知の `[Rare]` などはリテラル |

`max_length` は**ルーン数の予備警告**で、描画後のピクセル溢れを示すものではありません。
実際のピクセル溢れは Godot 表示レポートの `overflow` などとして別に取り込みます。
`<b>` などのタグも現状はルーン数に含みます。

壊れた翻訳は `sync` では適用されません。`sync` は1件でも検査に通らないと全体を
更新しないため、`targets/<locale>.json` の該当キーに訳文を手動で記載し、
`gamel10n approve -config gamel10n.json -locale <locale> -key <key>` で承認してから
もう一度 `sync` を実行してください。承認したキーは保持され、残りのキーだけが翻訳されます。
`check` は既存の翻訳データの壊れを報告するだけで、ファイルを書き換えません。

## Godot 連携（任意）

Godot は必須ではありません。`plan` / `sync` / `check` / `export` は JSON と CSV だけ
でも動きます。Godot を使う場合、`godot/` に Godot 4.3 以上を想定したアドオンと
独立したサンプルがあります。この CLI は `godot/` を生成・編集しません。

- アドオンが測定するのは、`metadata/l10n_key` を付けた `Label`、通常の `Button`、
  `RichTextLabel` だけです。
- アドオンの 1 回の検査は、指定したシーンのサブツリーと 1 ロケールが対象です。
  サンプルのランナーは ja と en を順に検査します。全シーン、全 Control、カスタム描画、
  `LineEdit` / `TextEdit` / `Label3D`、3D 世界は対象外です。
- `checked` はアドオンが数えたノード数です。全シーンを検査したことを示す値では
  ありません。スクリーンショットは任意の補助的な記録です。
- `clipped` / `overflow` は表示エラー（終了コード 4）、`missing_glyph` /
  `missing_key` / `unsupported_control` / `unsupported_mode` は警告として扱います。
- ヘッドレスで測る場合は、先に Godot の import を通します。レポートの出力先には
  `user://` か絶対パスを指定してください（`res://` は拒否されます）。生成方法の例は
  [godot/README.md](../godot/README.md) にあります。

```sh
godot --headless --path godot --import
```

表示レポートは次の形です。

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

生成したレポートを `reports/godot-display.json` に置いた場合、CLI では次のように
取り込みます。

```sh
gamel10n check -config gamel10n.json \
  -display-report reports/godot-display.json \
  -report reports/check.json
```

`clipped` / `overflow` だけが表示エラーで、それ以外の issue は警告です。対応するControl、
`kind`、確認済みの範囲と制限は [godot/README.md](../godot/README.md) と
[docs/godot-integration.md](godot-integration.md) にあります。

## 設定・スキーマ・レポート

完全なフィールド説明は [docs/schemas.md](schemas.md) にあります。

- 設定: `gamel10n.json`
- 原文データ: `source/catalog.json`
- 翻訳データ: `targets/<locale>.json`（フラットな `key -> string`）
- 状態: `state/gamel10n-state.json`（`schema_version: 1`）
- レポート: `plan` / `sync` / `check` の JSON（すべて `schema_version: 1`）

`source_hash` は原文テキストのみの SHA-256、`target_hash` は翻訳テキストの
SHA-256 です。`context` と `max_length` の変更だけでは `changed` になりません
（`max_length` は `check` の警告で拾います）。必要なら原文を少し変えるか、
状態を手動で更新してください。

## 終了コード

| コード | 意味 |
| --- | --- |
| 0 | 成功。`check` は警告のみも 0 |
| 1 | 設定・I/O・API・ロック・検証の失敗 |
| 2 | 引数/使い方のエラー |
| 3 | `check` のプレースホルダー / タグ構造エラー |
| 4 | `check` で取り込んだ表示エラーのみ（構造エラーなし） |

`check` は既定で JSON を stdout、要約を stderr に出します。
`-report PATH` を指定すると JSON をファイルに書き、要約を stdout に出します。

## 保存の順序と保護

`sync` は次の順で動きます。

1. `.gamel10n.lock` を排他作成。既にあれば失敗
2. 設定・原文データ・全翻訳データ・状態を読み、計画を作る
3. 必要な翻訳だけ API に送り、結果をメモリ上で検証
4. 出力 JSON をメモリ上で構築し、パス/キー/ロケールを再検証
5. 翻訳データと状態を一時ファイルに書き、元ファイルをバックアップ
6. 直前に原文データ・設定・翻訳データ・状態のハッシュを再確認
7. すべて置換。失敗時はバックアップを戻して元に戻す

外部エディタが同時に翻訳データを変更した場合は「changed during sync」で中止し、
その外部変更を上書きしません。ロックはこの CLI の `sync` 同士を守るもので、
外部エディタを強制ロックするものではありません。

レポートは翻訳データと状態の保存とは別のファイルに書きます。レポートの書き込み
だけが失敗した場合、翻訳データの保存自体は成功している可能性があります。
終了コード 1 とメッセージを確認してください。
