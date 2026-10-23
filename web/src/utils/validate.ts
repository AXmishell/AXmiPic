import type { FormItemRule } from 'element-plus'

import { translate } from '@/i18n'

/** 用户名规则：与后端一致，允许字母、数字与 '.', '_', '-', '+', '@'，长度 3-64。 */
export const USERNAME_PATTERN = /^[a-zA-Z0-9._+@-]{3,64}$/

/** 用户名不合规时的提示（按当前语言生成）。 */
export function usernameRuleMessage(): string {
  return translate('validation.usernamePattern')
}

/** 构造与后端一致的用户名校验规则。 */
export function usernameRules(requiredMessage?: string): FormItemRule[] {
  return [
    {
      required: true,
      message: requiredMessage ?? translate('validation.usernameRequired'),
      trigger: 'blur',
    },
    { min: 3, max: 64, message: translate('validation.usernameLength'), trigger: 'blur' },
    { pattern: USERNAME_PATTERN, message: usernameRuleMessage(), trigger: 'blur' },
  ]
}

/** 构造「确认密码」校验规则：非空且与主密码一致。 */
export function confirmPasswordRule(
  getPassword: () => string,
  emptyMessage?: string,
): FormItemRule {
  return {
    validator: (_rule: unknown, value: string, callback: (error?: Error) => void): void => {
      if (!value) {
        callback(new Error(emptyMessage ?? translate('validation.passwordConfirmRequired')))
        return
      }
      if (value !== getPassword()) {
        callback(new Error(translate('validation.passwordMismatch')))
        return
      }
      callback()
    },
    trigger: 'blur',
  }
}
