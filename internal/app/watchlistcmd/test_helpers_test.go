package watchlistcmd

type errorWriter struct {
	err error
}

func (writer errorWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

func withoutFlag(arguments []string, name string) []string {
	result := append([]string{}, arguments...)
	for index := range result {
		if arguments[index] == name {
			return append(result[:index], result[index+2:]...)
		}
	}
	return result
}

func replaceFlag(arguments []string, name, value string) []string {
	result := append([]string{}, arguments...)
	for index := range result {
		if result[index] == name {
			result[index+1] = value
			return result
		}
	}
	return result
}
