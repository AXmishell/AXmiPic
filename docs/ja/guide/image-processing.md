# 画像処理

画像は `/i/<storage-key>` でアクセスでき、クエリ文字列に処理パラメータを追加できます:

```
http://localhost:8080/i/2026/10/05/6f1c…a2.png?w=400&h=300&fit=cover&f=webp&q=80
```

| パラメータ | 意味 | 値 |
|-----------|---------|--------|
| `w` | 対象の幅 | ピクセル |
| `h` | 対象の高さ | ピクセル |
| `fit` | リサイズ方式 | `contain`（デフォルト、比率を保って拡大縮小）/ `cover`（埋めるようにトリミング）/ `fill`（埋めるように引き伸ばし。`cover`/`fill` はいずれも `w` と `h` が必要） |
| `q` | 出力品質 | 1〜100、デフォルトは `processing.default_quality` |
| `f` | 出力フォーマット | `jpeg`/`png`/`gif`/`webp`/`avif`（`processing.allowed_formats` とプロセッサの機能により制限されます） |
| `r` | 回転角度 | `90`/`180`/`270` |
| `flip` | 反転 | `h`（水平）/ `v`（垂直）/ `hv`（両方） |
| `gray` | グレースケールに変換 | `1`/`true` |
| `blur` | ガウスぼかしの半径 | 0〜100 |
| `sharpen` | シャープ化の強さ | 0〜100 |
| `wm` | テキスト透かしの内容 | 任意のテキスト（最大 200 文字） |
| `wm_pos` | 透かしの位置 | `top-left`/`top-right`/`bottom-left`/`bottom-right`/`center` |
| `wm_opacity` | 透かしの不透明度 | 0〜100（デフォルト 80） |
| `wm_size` | 透かしのフォントサイズ | ピクセル、0 は自動 |
| `wm_color` | 透かしの色 | 16 進数、例 `#ffffff` |
| `enlarge` | 拡大を許可するか | `1`/`true`/`yes`/`on` |

処理パラメータを付けたレスポンスには `ETag` が含まれ、`If-None-Match` による `304` 応答に対応します。

`processing` 設定ではスイッチとデフォルト値を指定します:

```yaml
processing:
  enabled: true
  max_width: 4096
  max_height: 4096
  default_quality: 82
  allowed_formats: ["jpeg", "png", "gif", "webp", "avif"]
  allow_enlarge: false      # 拡大を許可するか
  allow_effects: true       # グレースケール/ぼかし/シャープ化を許可するか
  allow_watermark: true     # URL から透かしを追加できるか
  watermark_text: ""        # 空でない場合、この透かしがすべての変換に強制的に適用される
```

フロントエンドの「画像処理」ページ（`/processing`）では、上記パラメータのライブプレビューを確認でき、コピー可能な処理リンクを生成できます。

## libvips プロセッサの使用（オプション）

デフォルトでは純粋な Go プロセッサが使用されます（JPEG/PNG/GIF 出力に対応）。WebP/AVIF 出力とより高いパフォーマンスを得るには、libvips をインストールして次のようにビルドします:

```bash
go build -tags libvips -o bin/axmipic ./cmd/axmipic
```

## 画像処理ドライバー

`processing.driver` は実行時に使用するプロセッサを選択します。不明または利用できないドライバーは起動時に `purego` へフォールバックします:

| ドライバー | 説明 | 要件 |
|--------|-------------|--------------|
| `purego` | 純粋な Go（デフォルト）、JPEG/PNG/GIF 出力に対応 | なし |
| `libvips` | libvips/bimg、WebP/AVIF にも対応し、パフォーマンスが高い | `-tags libvips` + CGO + libvips |
| `magick` | ImageMagick コマンドライン、WebP/AVIF にも対応 | システムに `magick` または `convert` がインストールされていること |

`GET /api/v1/admin/imaging/drivers` は、現在のバイナリで利用可能なドライバーと使用中のプロセッサを返します。
