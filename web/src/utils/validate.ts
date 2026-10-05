import type { FormItemRule } from 'element-plus'

/** 用户名规则：与后端一致，允许字母、数字与 '.', '_', '-', '+', '@'，长度 3-64。 */
export const USERNAME_PATTERN = /^[a-zA-Z0-9._+@-]{3,64}$/

/** 用户名不合规时的提示。 */
export const USERNAME_RULE_MESSAGE = '用户名仅可包含字母、数字与 "."、"_"、"-"、"+"、"@"，长度 3-64 位'

/** 构造与后端一致的用户名校验规则。 */
export function usernameRules(requiredMessage = '请输入用户名'): FormItemRule[] {
  return [
    { required: true, message: requiredMessage, trigger: 'blur' },
    { min: 3, max: 64, message: '用户名长度为 3 到 64 个字符', trigger: 'blur' },
    { pattern: USERNAME_PATTERN, message: USERNAME_RULE_MESSAGE, trigger: 'blur' },
  ]
}
