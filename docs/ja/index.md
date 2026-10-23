---
layout: home

hero:
  name: AXmiPic
  text: 軽量で信頼性の高いセルフホスト型画像ホスティングサービス
  tagline: 単一の Go バイナリ + 組み込みのシングルページアプリケーション。複数のストレージバックエンド、オンザフライの画像処理、アルバムと画像広場、プラン課金、管理者コンソールをサポートします。
  image:
    src: /favicon.svg
    alt: AXmiPic
  actions:
    - theme: brand
      text: はじめに
      link: /ja/guide/getting-started
    - theme: alt
      text: API リファレンス
      link: /ja/api/
    - theme: alt
      text: GitHub
      link: https://github.com/AXmishell/AXmiPic

features:
  - title: 複数のアップロード方法
    details: 管理者 UI からのアップロード、バッチアップロード、貼り付けアップロード、ドラッグ＆ドロップアップロード、multipart API、オブジェクトストレージへの署名付き直接アップロードに対応しています。
  - title: オンザフライの画像処理
    details: URL クエリパラメータによるリアルタイムのリサイズ、トリミング、回転、反転、グレースケール、ぼかし、シャープ化、テキスト透かし、フォーマット変換を、ETag キャッシュ付きで提供します。
  - title: 複数のストレージバックエンド
    details: ローカルファイルシステム、S3 互換オブジェクトストレージ（AWS / MinIO / R2 / OSS / COS）、Qiniu Cloud Kodo に対応し、管理者コンソールから実行中にホットスイッチできます。
  - title: アカウントと権限
    details: JWT セッション + 長期有効な API トークン。ロールグループはポリシーを通じてクォータ、レート制限、画像処理機能、機能トグルを制御し、Guest ロールを標準搭載しています。
  - title: サイト運用
    details: アルバムと画像広場、共有リンク、お知らせと単独ページ、通報対応、チケットシステム、AI 画像モデレーション。
  - title: プランと決済
    details: プラン、クーポン、注文、およびプラグイン可能な決済チャネル（Alipay 対面決済、WeChat Pay v3、YiPay、手動、モック）。
---
