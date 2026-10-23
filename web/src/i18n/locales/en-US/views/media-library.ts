export default {
  processing: {
    title: 'Image processing',
    description:
      'Resize, crop, rotate, flip, filter and watermark images in real time using URL parameters.',
    actions: {
      resetParams: 'Reset parameters',
      copyLink: 'Copy link',
    },
    empty: {
      title: 'No images to process yet',
      description: 'Upload an image first, then come back here to fine-tune the processing parameters.',
    },
    labels: {
      selectImage: 'Select image',
      width: 'Width',
      height: 'Height',
      fit: 'Fit',
      format: 'Output format',
      quality: 'Quality',
      rotate: 'Rotate',
      flip: 'Flip',
      enlarge: 'Allow upscaling',
      grayscale: 'Grayscale',
      blur: 'Blur strength',
      sharpen: 'Sharpen strength',
      watermark: 'Text watermark',
      watermarkText: 'Watermark text',
      position: 'Position',
      opacity: 'Opacity',
      fontSize: 'Font size (0 = auto)',
      color: 'Color',
    },
    format: {
      keepOriginal: 'Keep original format',
    },
    rotate: {
      none: 'No rotation',
    },
    flip: {
      none: 'No flip',
      horizontal: 'Horizontal',
      vertical: 'Vertical',
      both: 'Horizontal + vertical',
    },
    watermarkPlaceholder: 'e.g. AXmiPic',
    preview: {
      title: 'Live preview',
      alt: 'Preview',
      unavailable: 'Preview unavailable. Check the processing parameters.',
      linkLabel: 'Processed link',
    },
    messages: {
      copied: 'Processed link copied',
      copyFailed: 'Copy failed',
    },
  },
  albums: {
    title: 'Albums',
    description: 'Group images into albums for easier management and browsing.',
    create: 'New album',
    empty: {
      title: 'No albums yet',
      description: 'Create an album, then assign images to it from the image manager.',
    },
    dialog: {
      createTitle: 'New album',
      editTitle: 'Edit album',
    },
    form: {
      name: 'Name',
      namePlaceholder: 'e.g. Travel / Wallpapers',
      intro: 'Introduction',
      introPlaceholder: 'Optional',
      visibility: 'Visibility',
      visibilityHint:
        'Once public, anyone can browse the album and its images without signing in.',
    },
    visibility: {
      private: 'Private',
      public: 'Public',
    },
    columns: {
      name: 'Name',
      imageCount: 'Images',
      visibility: 'Visibility',
      intro: 'Introduction',
      createdAt: 'Created',
    },
    actions: {
      viewImages: 'View images',
      share: 'Share',
    },
    validation: {
      nameRequired: 'Enter an album name',
      nameLength: 'Name must be 1 to 100 characters long',
      introMax: 'Introduction must be at most 255 characters',
    },
    confirm: {
      deleteTitle: 'Delete album',
      deleteMessage:
        'Deleting the album "{name}" keeps its images, but they will no longer belong to any album.',
    },
    messages: {
      updated: 'Album updated',
      created: 'Album created',
      deleted: 'Album deleted',
      shareDisabled: 'Sharing is not enabled for your role',
    },
  },
  plaza: {
    title: 'Image plaza',
    description: 'Images shared publicly by all users.',
    searchPlaceholder: 'Search file names',
    order: {
      newest: 'Newest',
      earliest: 'Earliest',
      largest: 'Largest',
      smallest: 'Smallest',
    },
    layout: {
      switchToGrid: 'Switch to grid',
      switchToMasonry: 'Switch to masonry',
      grid: 'Grid',
      masonry: 'Masonry',
    },
    count: '{count} public images',
    listLabel: 'Image plaza',
    actions: {
      copyLink: 'Copy link',
    },
    empty: {
      noMatchTitle: 'No matching images',
      noMatchDescription: 'Try another keyword or clear the search.',
      noItemsTitle: 'No public images in the plaza yet',
      noItemsDescription:
        'Set an image to "Public" in the image manager and it will appear here.',
      clearSearch: 'Clear search',
    },
    messages: {
      copied: 'Copied',
      copyFailed: 'Copy failed. Please copy manually.',
    },
  },
  shares: {
    title: 'My shares',
    description: 'Manage share links for images and albums, and revoke them at any time.',
    empty: {
      title: 'No share links yet',
      description: 'Click "Share" in the image manager or on an album to create a link.',
    },
    type: {
      album: 'Album',
      image: 'Image',
    },
    columns: {
      type: 'Type',
      link: 'Link',
      password: 'Password',
      views: 'Views',
      createdAt: 'Created',
    },
    passwordRequired: 'Required',
    actions: {
      revoke: 'Revoke',
    },
    confirm: {
      revokeTitle: 'Revoke share',
      revokeMessage: 'The link will stop working immediately. This cannot be undone.',
    },
    messages: {
      revoked: 'Share revoked',
    },
  },
  plazaCard: {
    previewUnavailable: 'Preview unavailable',
    authorLabel: 'By:',
    uploader: 'Uploaded by {name}',
    anonymous: 'Anonymous',
    meta: {
      dimensions: 'Dimensions',
      size: 'Size',
      format: 'Format',
      uploadedAt: 'Uploaded',
    },
  },
  plazaMasonry: {
    empty: 'No public images yet',
    previewUnavailable: 'Preview unavailable',
    authorLabel: 'By:',
    uploader: 'Uploaded by {name}',
    anonymous: 'Anonymous',
    loadingMore: 'Loading more…',
    end: 'You have reached the end',
  },
}
