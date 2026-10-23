import common from './common'

type MessageTree = { [key: string]: string | MessageTree }

/** ビュー単位の文言は `./views/*.ts` に分割し、追加時に自動でマージされます。 */
const viewModules = import.meta.glob<{ default: MessageTree }>('./views/*.ts', {
  eager: true,
})

const views = Object.values(viewModules).reduce<MessageTree>((acc, module) => {
  return { ...acc, ...module.default }
}, {})

const messages: MessageTree = { ...common, ...views }

export default messages
