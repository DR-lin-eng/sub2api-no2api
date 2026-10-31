export type ModelAttributeFieldType = 'string' | 'integer' | 'number' | 'boolean' | 'enum'

export interface ModelAttributeField {
  key: string
  type: ModelAttributeFieldType
  values?: string[]
  default?: string | number | boolean
  required?: boolean
  min?: number
  max?: number
  ui?: 'auto' | 'hidden'
  label?: string
}

export type ModelAttributeValues = Record<string, string | number | boolean>

// replace：只渲染模板声明的字段；overlay：在硬编码参数之上用同名字段替换对应控件。
export type ModelAttributeRenderMode = 'replace' | 'overlay'

export interface ModelAttributeBlock {
  mode?: ModelAttributeRenderMode
  fields?: ModelAttributeField[]
  // hide 中的 key 对应的宿主内置控件不渲染、也不提交，用于屏蔽模型不支持的参数。
  hide?: string[]
}
