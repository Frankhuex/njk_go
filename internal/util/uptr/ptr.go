package uptr

func EmptyToNil(value string) *string {
	if value == "" {
		return nil
	}
	copyValue := value
	return &copyValue
}

