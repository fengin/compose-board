package filelog

// ValidateFollow 在写出 SSE 响应头之前校验根目录、相对路径、文件类型和 tail 参数。
func (m *Manager) ValidateFollow(rootID string, relativePath string, tail int) error {
	if _, err := m.getFollowBase(rootID, relativePath); err != nil {
		return err
	}
	_, err := normalizeTail(tail)
	return err
}
