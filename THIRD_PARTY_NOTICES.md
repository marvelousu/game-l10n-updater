# Third-party notices

## PLaMo 3 Translate

This project is **Built with PLaMo** through the separate
[local-jp-translate](https://github.com/marvelousu/local-jp-translate) API and
the PLaMo 3 Translate model. This repository does **not** bundle model weights,
llama.cpp, or the translation server.

The upstream model and weights are distributed separately:

- Model card and weights: <https://huggingface.co/pfnet/plamo-3-translate-0.8b-GGUF>
- PLaMo Community License (Japanese): <https://plamo.preferredai.jp/info/plamo-community-license-ja>
- PLaMo Community License (English): <https://plamo.preferredai.jp/info/plamo-community-license-en>
- Commercial use registration: <https://forms.gle/mTL8tBLrMYXKNZD56>

The **source code of this repository is MIT-licensed**. Model weights and the
PLaMo Community License are separate; review the model terms and complete any
required commercial registration yourself. This project does not accept model
terms on your behalf and does not automate registration.

## local-jp-translate

`local-jp-translate` is a separate project that runs the local translation API.
Its source code and notices are at
<https://github.com/marvelousu/local-jp-translate>. Its README describes the
model download and llama-server setup. This project only calls its loopback
`POST /translate` endpoint.

## Go standard library

The CLI uses only the Go standard library and is built with the Go toolchain.
See <https://go.dev/LICENSE>.
