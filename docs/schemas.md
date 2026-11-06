# 設定とデータファイルのスキーマ

すべて UTF-8、`schema_version` は現行 `1` です。JSON は厳格に読みます
（未知フィールド、重複キー、末尾の余分な値はエラー）。書き出しは 2 スペース
インデント、末尾改行、キーソート、HTML エスケープなしです。

## 1. `gamel10n.json`

```json
{
  "schema_version": 1,
  "source": {
    "language": "ja",
    "path": "source/catalog.json"
  },
  "targets": {
    "directory": "targets",
    "state": "state/gamel10n-state.json"
  },
  "languages": ["en", "zh-hans"],
  "api": {
    "url": "http://127.0.0.1:8787",
    "style": "",
    "timeout_seconds": 210,
    "max_tokens": 512
  },
  "glossary": [
    { "source": "ポーション", "target": "Potion", "locale": "en" }
  ]
}
```

| フィールド | 必須 | 説明 |
| --- | --- | --- |
| `schema_version` | はい | `1` 固定 |
| `source.language` | はい | 原文の言語コード。`auto` は不可 |
| `source.path` | はい | プロジェクトルートからの相対パス。`..`、絶対パス、ドライブ指定は不可 |
| `targets.directory` | はい | ターゲット JSON を置くディレクトリ。相対パス |
| `targets.state` | はい | 状態ファイル。相対パス |
| `languages` | はい | ターゲット言語コードの配列。重複と原文言語の混入は不可 |
| `api.url` | はい | `http://localhost:8787`、`http://127.0.0.1:8787`、`http://[::1]:8787` などのループバックのみ |
| `api.style` | いいえ | `default` / `polite` / `plain`（空も可）。ターゲットが `ja` のときだけ API に送り、それ以外では送らない |
| `api.timeout_seconds` | いいえ | 0 または省略時は 210。1〜3600。サーバー側の既定は 180 秒 |
| `api.max_tokens` | いいえ | 0 または省略時は 512。負数は不可 |
| `glossary` | いいえ | `source`/`target` は必須。`locale` が空なら全ロケール、指定時はそのロケールだけ |

言語コードはファイル名に使える `[A-Za-z0-9][A-Za-z0-9_-]*`（最大 64）です。
ロケール名と `targets/<locale>.json` のファイル名は設定した表記のまま保持します。
API へ送るときだけ小文字化し、`_` を `-` に直します。`zh-cn` / `zh-sg` は
`zh-hans`、`zh-tw` / `zh-hk` は `zh-hant` として送ります。`pt-BR` のような
その他の地域付きコードは変換しません。

`targets.directory` 内のファイル名は `<locale>.json` に固定です。ロケールは
検証済みなので `../` などでプロジェクト外に出ることはありません。

## 2. 原文データ

配列形式:

```json
[
  { "key": "ui.start", "text": "はじめから", "context": "title menu" },
  { "key": "ui.damage", "text": "ダメージ: %d", "context": "battle", "max_length": 16 }
]
```

オブジェクト形式:

```json
{
  "schema_version": 1,
  "entries": [
    { "key": "ui.start", "text": "はじめから", "context": "title menu" }
  ]
}
```

| フィールド | 必須 | 説明 |
| --- | --- | --- |
| `key` | はい | 空でない安定 ID。重複不可。ファイルパスには使わない |
| `text` | はい | 原文。空文字も許容するが、API が拒否する場合がある |
| `context` | いいえ | 翻訳 API の `context` に渡す |
| `max_length` | いいえ | 正の整数。翻訳後のルーン数警告に使う。0/省略は無効 |

配列形式には `schema_version` がありません。オブジェクト形式では `1` が必須です。
原文データは `sync` が書き換えません。`import-csv` だけが明示的に生成します。

### 原文ハッシュ

`source_hash = SHA-256(text)` です。`context` と `max_length` は含みません。
そのため、これらだけを変更しても `plan` は `changed` にしません。

## 3. 翻訳データ（フラット JSON）

`targets/en.json`:

```json
{
  "item.potion": "You got a potion.",
  "ui.damage": "Damage: %d",
  "ui.greet": "Hello, {name}!"
}
```

- トップレベルはオブジェクト、値は文字列のみ。
- キーはソースの `key` と対応する。ソースにないキーは `deleted` として報告され、
  既定では保持される。
- 重複キー、数値/配列/null 値、末尾の余分な JSON はエラー。
- キーは書き出し時にソート。値の Unicode、`<b>`、`[color=red]` などは
  エスケープせずそのまま。
- ファイルが存在しない場合は空の翻訳データとして扱う。`sync` が必要時に作る。

既存の翻訳データは編集してよい。状態の `target_hash` と違えば手動編集とみなす。

## 4. 状態 `state/gamel10n-state.json`

```json
{
  "schema_version": 1,
  "files": {
    "en": {
      "entries": {
        "ui.greet": {
          "source_hash": "9f2c...64桁...",
          "target_hash": "77aa...64桁...",
          "manual": true,
          "approved": true,
          "locked": false,
          "needs_review": false
        }
      }
    }
  }
}
```

| フィールド | 説明 |
| --- | --- |
| `files.<locale>.entries.<key>.source_hash` | 最後に翻訳/承認した原文の SHA-256。空も許容 |
| `target_hash` | 最後に生成/承認/編集検出したターゲットの SHA-256。空も許容 |
| `manual` | 手動の翻訳。`sync` は上書きしない |
| `approved` | 承認済み。上書きしない |
| `locked` | 固定。上書きしない |
| `needs_review` | 原文変更後に保護対象として保持した。`approve` するまで残る |

- ハッシュは小文字 64 桁の SHA-256。空文字は「未記録」として許容します。
- 3 つの保護フラグのいずれかが立っていれば、`sync` は値を上書きしません。
- 原文が変わった保護対象は `needs_review: true` にし、`source_hash` は古いまま
  残します。`target_hash` は現在値に更新します。
- `approve` は `source_hash`/`target_hash` を現在値に更新し、`needs_review` を
  消し、`manual`/`approved` を立てます。`-lock` で `locked` も立てます。
- `unlock` は `locked` だけを下ろします。
- 状態は手動で編集しても読めます。ただしハッシュ形式・ロケール名・キーは検証されます。
- 状態を失うと、既存ターゲットはすべて「初回の手動訳」として `manual` に
  なります。安全側ですが、機械生成の再開には `approve` ではなく状態の再構築が
  必要です。

## 5. plan レポート

```json
{
  "schema_version": 1,
  "kind": "plan",
  "source_language": "ja",
  "locales": ["en"],
  "actions": [
    {
      "key": "ui.greet",
      "locale": "en",
      "action": "changed",
      "reason": "source text changed",
      "source_text": "こんにちは、{name}さん！",
      "target_text": "Hello, {name}!",
      "source_hash": "...",
      "target_hash": "...",
      "manual": false,
      "approved": false,
      "locked": false,
      "needs_review": false
    }
  ],
  "counts": {
    "new": 0,
    "changed": 1,
    "unchanged": 4,
    "deleted": 0,
    "needs_review": 0
  },
  "api_requests": 1
}
```

`actions` は `locale` → `key` でソートされます。`action` は
`new` / `changed` / `unchanged` / `deleted` / `needs_review` のいずれかです。
`plan` は API を呼びません。

## 6. sync レポート

```json
{
  "schema_version": 1,
  "kind": "sync",
  "status": "success",
  "source_language": "ja",
  "locales": ["en"],
  "actions": [],
  "counts": {
    "new": 1,
    "changed": 0,
    "unchanged": 2,
    "deleted": 0,
    "needs_review": 0
  },
  "api_calls": 1,
  "translations": [
    {
      "key": "ui.greet",
      "locale": "en",
      "translated_text": "Hello, {name}!",
      "applied": true
    }
  ],
  "warnings": [],
  "wrote_files": true,
  "pruned": 0
}
```

- `status`: `success` / `no_changes` / `failed`。
- `wrote_files`: ターゲットまたは状態を実際に置換したか。
- `translations[].applied`: その結果が最終的にコミットされたか。失敗時は false。
- `rejected`: プレースホルダー/タグ検証で拒否した翻訳。`translated_text` と
  `issues` を含む。適用しない。
- `warnings`: `max_length` など。適用は止めない。
- `error`: 失敗理由。
- `pruned`: `--prune-deleted` で消したキー数。
- 成功・失敗ともレポートはターゲット/状態とは別ファイル。

同期の途中で API 失敗・検証失敗・同時変更・commit 失敗が起きた場合、
`status: failed`、`wrote_files: false`、`translations[].applied: false` になり、
ターゲットと状態は元のバイト列のままです。

## 7. check レポート

```json
{
  "schema_version": 1,
  "kind": "check",
  "source_language": "ja",
  "locales": ["en"],
  "issues": [
    {
      "key": "ui.greet",
      "locale": "en",
      "kind": "placeholder_mismatch",
      "severity": "error",
      "message": "brace placeholder multiset differs: missing {name}",
      "category": "brace",
      "source": "catalog",
      "missing": ["{name}"]
    }
  ],
  "counts": {
    "errors": 1,
    "warnings": 0,
    "structural": 1,
    "display": 0
  },
  "checked": 5,
  "display_report": {
    "path": "reports/godot-display.json",
    "checked": 42,
    "issues": 1,
    "unknown_keys": [],
    "unknown_locales": []
  }
}
```

- `kind`: `placeholder_mismatch`（構造エラー）、`max_length`（警告）、
  `missing_target`（警告）、または取り込んだ表示レポートの `kind`。
- 表示レポートの `clipped` / `overflow` は `severity: error`（終了コード 4）に、
  `missing_glyph` / `missing_key` / `unsupported_control` / `unsupported_mode`
  などは `severity: warning` にマップします。確定した描画溢れだけをエラー扱いします。
- `source`: `catalog` または `display`。
- `checked`: `locales × ソースキー数`。
- 表示レポートの issue は `node_path` を保持します。ソースにないキーは
  `unknown_key: true` と `display_report.unknown_keys` に、source/target の設定に
  ないロケールは `unknown_locale: true` と `display_report.unknown_locales` に出ます。
  `-locale` で絞り込んだ場合は、そのロケールの表示 issue だけを取り込みます。
- `check` はファイルを変更しません。

## 8. Godot 表示レポート（外部入力）

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

`schema_version` は `1` 固定。`issues` の `key` / `locale` / `kind` は必須、
`node_path` / `message` は任意です。`checked` は 0 以上。未知フィールドは拒否します。
`checked` は検査した件数です。検査対象は、アドオンで指定したシーン内のUI部品です。

## 9. 翻訳 API

リクエスト:

```json
{
  "text": "こんにちは、{name}さん！",
  "source": "ja",
  "target": "en",
  "glossary": [
    { "source": "ポーション", "target": "Potion" }
  ],
  "context": "title screen greeting",
  "max_tokens": 512
}
```

`style` はターゲットが `ja` で設定値があるときだけ含めます。`glossary` は
該当ロケール + 全ロケール共通の用語だけを配列で送ります（該当なしは `[]`）。
`source` と `target` は上記の規則で API 用の表記に正規化します。

`sync` は翻訳が必要なとき、最初に `GET /languages` を1回呼びます。設定した
原文言語と今回翻訳するターゲット言語が API の一覧にない場合は、翻訳を始める前にエラーにします。
`plan` はこの確認も API 呼び出しも行いません。

成功レスポンス（HTTP 2xx）:

```json
{ "translated_text": "Hello, {name}!" }
```

`translated_text` は必須で、空/空白のみは失敗として扱います。HTTP 非 2xx は
`{"error":{"code":"...","message":"..."}}` を読み、同期全体を失敗させます。

## 10. Godot CSV

```csv
keys,ja,en,zh-hans
ui.start,はじめから,Start,开始
"ui.quote","カンマ, と引用","Comma, and quote","逗号, 和引号"
```

- 1 行目はヘッダー。先頭列は `keys`（`key` も可）。
- `-source-language` の列が原文。他列がターゲット。
- 標準 CSV のクォート、カンマ、改行、Unicode をそのまま扱う。BOM は入力時に
  先頭ヘッダーから除去し、出力には付けない。
- 行のキーは空不可、重複不可。列数はヘッダーと一致必須。
- import の空ターゲットセルは「未翻訳」として省略する。手動の空文字翻訳を
  表現したい場合は JSON を直接編集する。

## 11. 決定性

- ソースはキー順、ターゲット/状態はマップキー順、レポートはロケール→キー順。
- レポートと状態に時刻・乱数・ホスト名を入れない。
- 一時ファイル名とロック内容だけは実行時に生成されるが、成果物には残らない
  （ロックは終了時に削除）。
