package helpers

// simple check if a string is present in a slice
func StringInSlice(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}
