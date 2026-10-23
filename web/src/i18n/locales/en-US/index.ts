import common from './common'

type MessageTree = { [key: string]: string | MessageTree }

/** 视图级文案按模块拆分在 `./views/*.ts`，新增模块后自动合并。 */
const viewModules = import.meta.glob<{ default: MessageTree }>('./views/*.ts', {
  eager: true,
})

const views = Object.values(viewModules).reduce<MessageTree>((acc, module) => {
  return { ...acc, ...module.default }
}, {})

const messages: MessageTree = { ...common, ...views }

export default messages
