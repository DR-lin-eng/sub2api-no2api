package service

// openAIImagesAttributeValues 把图片请求里客户端已提交的字段值映射成通用 map。
// 这里只做取值映射，不做校验、不做改写；JSON 请求由 applyModelAttributes 自行解析。
func openAIImagesAttributeValues(parsed *OpenAIImagesRequest) map[string]any {
	if parsed == nil {
		return nil
	}
	values := make(map[string]any, 3)
	if parsed.Size != "" {
		values["size"] = parsed.Size
	}
	if parsed.Quality != "" {
		values["quality"] = parsed.Quality
	}
	if parsed.N > 0 {
		values["n"] = parsed.N
	}
	return values
}
