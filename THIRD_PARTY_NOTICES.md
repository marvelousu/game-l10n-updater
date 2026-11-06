# サードパーティの表記

## PLaMo 3 Translate

翻訳には [local-jp-translate](https://github.com/marvelousu/local-jp-translate) API を通じて PLaMo 3 Translate を利用しています（Built with PLaMo）。モデル重み、llama.cpp、翻訳サーバーは別途用意します。

モデルと重みは別に配布されています。

- モデルカードと重み: <https://huggingface.co/pfnet/plamo-3-translate-0.8b-GGUF>
- PLaMo Community License（日本語）: <https://plamo.preferredai.jp/info/plamo-community-license-ja>
- PLaMo Community License（英語）: <https://plamo.preferredai.jp/info/plamo-community-license-en>
- 商用利用の登録: <https://forms.gle/mTL8tBLrMYXKNZD56>

ソースコードはMITライセンス、モデル重みはPLaMo Community Licenseで提供されます。個人・趣味・学術などの非商用利用は無償で、登録は不要です。商用利用にはPFNへの事前登録と売上規模の条件があり、必要な手続きは利用者自身で行います。条件の全文は上記のライセンスを参照してください。

## local-jp-translate

`local-jp-translate` は、ローカル翻訳APIを動かす別プロジェクトです。ソースコードと表記は <https://github.com/marvelousu/local-jp-translate> にあります。READMEにモデルのダウンロードとllama-serverの設定方法があります。このプロジェクトは、そのループバック `POST /translate` エンドポイントを呼ぶだけです。

## Go標準ライブラリ

CLIはGo標準ライブラリだけを使い、Goツールチェーンでビルドします。<https://go.dev/LICENSE> を参照してください。
