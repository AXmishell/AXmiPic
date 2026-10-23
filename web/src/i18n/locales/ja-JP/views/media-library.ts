export default {
  processing: {
    title: '画像処理',
    description:
      'URL パラメータを使用して、画像のリサイズ、切り抜き、回転、反転、フィルター、透かしをリアルタイムで行います。',
    actions: {
      resetParams: 'パラメータをリセット',
      copyLink: 'リンクをコピー',
    },
    empty: {
      title: '処理できる画像がまだありません',
      description:
        '先に画像をアップロードしてから、ここに戻って処理パラメータを調整してください。',
    },
    labels: {
      selectImage: '画像を選択',
      width: '幅',
      height: '高さ',
      fit: 'フィット',
      format: '出力形式',
      quality: '品質',
      rotate: '回転',
      flip: '反転',
      enlarge: '拡大を許可',
      grayscale: 'グレースケール',
      blur: 'ぼかしの強さ',
      sharpen: 'シャープの強さ',
      watermark: 'テキスト透かし',
      watermarkText: '透かしテキスト',
      position: '位置',
      opacity: '不透明度',
      fontSize: 'フォントサイズ（0 = 自動）',
      color: '色',
    },
    format: {
      keepOriginal: '元の形式を維持',
    },
    rotate: {
      none: '回転なし',
    },
    flip: {
      none: '反転なし',
      horizontal: '水平',
      vertical: '垂直',
      both: '水平 + 垂直',
    },
    watermarkPlaceholder: '例：AXmiPic',
    preview: {
      title: 'ライブプレビュー',
      alt: 'プレビュー',
      unavailable: 'プレビューを表示できません。処理パラメータを確認してください。',
      linkLabel: '処理後のリンク',
    },
    messages: {
      copied: '処理後のリンクをコピーしました',
      copyFailed: 'コピーに失敗しました',
    },
  },
  albums: {
    title: 'アルバム',
    description: '画像をアルバムにまとめると、管理と閲覧が簡単になります。',
    create: '新しいアルバム',
    empty: {
      title: 'アルバムがまだありません',
      description: 'アルバムを作成し、画像管理から画像を割り当ててください。',
    },
    dialog: {
      createTitle: '新しいアルバム',
      editTitle: 'アルバムを編集',
    },
    form: {
      name: '名前',
      namePlaceholder: '例：旅行 / 壁紙',
      intro: '紹介',
      introPlaceholder: '任意',
      visibility: '公開設定',
      visibilityHint:
        '公開すると、ログインしていなくても誰でもアルバムとその画像を閲覧できます。',
    },
    visibility: {
      private: '非公開',
      public: '公開',
    },
    columns: {
      name: '名前',
      imageCount: '画像',
      visibility: '公開設定',
      intro: '紹介',
      createdAt: '作成日時',
    },
    actions: {
      viewImages: '画像を表示',
      share: '共有',
    },
    validation: {
      nameRequired: 'アルバム名を入力してください',
      nameLength: '名前は1～100文字である必要があります',
      introMax: '紹介は255文字以内である必要があります',
    },
    confirm: {
      deleteTitle: 'アルバムを削除',
      deleteMessage:
        'アルバム「{name}」を削除しても画像は残りますが、どのアルバムにも属さなくなります。',
    },
    messages: {
      updated: 'アルバムを更新しました',
      created: 'アルバムを作成しました',
      deleted: 'アルバムを削除しました',
      shareDisabled: 'お使いのロールでは共有が有効になっていません',
    },
  },
  plaza: {
    title: '画像広場',
    description: 'すべてのユーザーが公開共有している画像です。',
    searchPlaceholder: 'ファイル名を検索',
    order: {
      newest: '新しい順',
      earliest: '古い順',
      largest: '大きい順',
      smallest: '小さい順',
    },
    layout: {
      switchToGrid: 'グリッドに切り替え',
      switchToMasonry: 'メイソンリーに切り替え',
      grid: 'グリッド',
      masonry: 'メイソンリー',
    },
    count: '{count} 枚の公開画像',
    listLabel: '画像広場',
    actions: {
      copyLink: 'リンクをコピー',
    },
    empty: {
      noMatchTitle: '一致する画像がありません',
      noMatchDescription: '別のキーワードを試すか、検索をクリアしてください。',
      noItemsTitle: '広場にはまだ公開画像がありません',
      noItemsDescription:
        '画像管理で画像を「公開」に設定すると、ここに表示されます。',
      clearSearch: '検索をクリア',
    },
    messages: {
      copied: 'コピーしました',
      copyFailed: 'コピーに失敗しました。手動でコピーしてください。',
    },
  },
  shares: {
    title: 'マイ共有',
    description: '画像とアルバムの共有リンクを管理し、いつでも無効化できます。',
    empty: {
      title: '共有リンクがまだありません',
      description: '画像管理またはアルバムで「共有」をクリックしてリンクを作成してください。',
    },
    type: {
      album: 'アルバム',
      image: '画像',
    },
    columns: {
      type: '種類',
      link: 'リンク',
      password: 'パスワード',
      views: '閲覧数',
      createdAt: '作成日時',
    },
    passwordRequired: '必須',
    actions: {
      revoke: '無効化',
    },
    confirm: {
      revokeTitle: '共有を無効化',
      revokeMessage: 'リンクはすぐに使用できなくなります。この操作は元に戻せません。',
    },
    messages: {
      revoked: '共有を無効化しました',
    },
  },
  plazaCard: {
    previewUnavailable: 'プレビューを表示できません',
    authorLabel: '作者：',
    uploader: '{name} がアップロード',
    anonymous: '匿名',
    meta: {
      dimensions: '寸法',
      size: 'サイズ',
      format: '形式',
      uploadedAt: 'アップロード日時',
    },
  },
  plazaMasonry: {
    empty: '公開画像がまだありません',
    previewUnavailable: 'プレビューを表示できません',
    authorLabel: '作者：',
    uploader: '{name} がアップロード',
    anonymous: '匿名',
    loadingMore: 'さらに読み込み中…',
    end: '最後まで表示しました',
  },
}
