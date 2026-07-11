package urlutil

import "strings"

func NormalizeBaseURL(u, version string) string {
	if u == "" {
		return ""
	}
	if before, ok := strings.CutSuffix(u, "#"); ok {
		return strings.TrimRight(before, "/")
	}
	if version == "" {
		return strings.TrimRight(u, "/")
	}
	if strings.HasSuffix(u, "/"+version) || strings.Contains(u, "/"+version+"/") {
		return strings.TrimRight(u, "/")
	}
	return strings.TrimRight(u, "/") + "/" + version
}

func BuildRequestURL(baseURL, defaultPath, endpointPath string, pathParams ...string) string {
	base := strings.TrimRight(baseURL, "/")
	if endpointPath != "" {
		return base + endpointPath
	}
	return base + defaultPath
}

func JoinURL(base string, parts ...string) string {
	result := strings.TrimRight(base, "/")
	for _, p := range parts {
		result += "/" + strings.Trim(p, "/")
	}
	return result
}
