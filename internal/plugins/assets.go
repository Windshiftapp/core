package plugins

import (
	"fmt"
	"sort"
	"strings"
)

// Asset policy helpers. Assets are served by the host, so their auth policy is
// declared separately from WASM routes but uses the same RouteAuth catalog and
// the same "default to session, public is an explicit opt-in" rule.

// normalizeAssets defaults an unset asset auth to session, rejects an unknown
// value, validates each path, and rejects two entries that can match the same
// asset with different auth. The result is sorted so the resolved policy and
// any diagnostics are deterministic.
func normalizeAssets(assets []Asset, allowPublic bool) ([]Asset, error) {
	normalized := make([]Asset, len(assets))
	for i, asset := range assets {
		if err := validateAssetPath(asset.Path); err != nil {
			return nil, err
		}
		switch asset.Auth {
		case "":
			asset.Auth = RouteAuthSession
		case RouteAuthNone:
			if !allowPublic {
				return nil, fmt.Errorf("asset %s declares auth %q but public plugin assets are disabled (set PLUGIN_PUBLIC_ROUTES=true to allow)", asset.Path, RouteAuthNone)
			}
		case RouteAuthSession, RouteAuthAdmin:
		default:
			return nil, fmt.Errorf("asset %s declares unknown auth %q", asset.Path, asset.Auth)
		}
		normalized[i] = asset
	}

	for i := range normalized {
		for j := i + 1; j < len(normalized); j++ {
			if normalized[i].Auth == normalized[j].Auth {
				continue
			}
			if routePatternsOverlap(normalized[i].Path, normalized[j].Path) {
				return nil, fmt.Errorf(
					"assets %s (%s) and %s (%s) can match the same asset with different auth",
					normalized[i].Path, normalized[i].Auth, normalized[j].Path, normalized[j].Auth)
			}
		}
	}

	sort.SliceStable(normalized, func(i, j int) bool {
		return normalized[i].Path < normalized[j].Path
	})
	return normalized, nil
}

// validateAssetPath rejects an asset policy path that could not map to a file
// under the plugin's assets directory, or that would be ambiguous to match.
func validateAssetPath(assetPath string) error {
	if assetPath == "" {
		return fmt.Errorf("asset path must not be empty")
	}
	if strings.HasPrefix(assetPath, "/") || strings.Contains(assetPath, "\\") {
		return fmt.Errorf("invalid asset path %q: must be a relative path", assetPath)
	}
	if strings.Count(assetPath, "*") > 1 || (strings.Contains(assetPath, "*") && !strings.HasSuffix(assetPath, "*")) {
		return fmt.Errorf("invalid asset path %q: '*' is only allowed as a trailing prefix wildcard", assetPath)
	}
	for _, segment := range strings.Split(assetPath, "/") {
		if segment == ".." {
			return fmt.Errorf("invalid asset path %q: must not contain '..'", assetPath)
		}
	}
	return nil
}

// assetAuth returns the auth policy for an asset path. An exact entry beats a
// prefix entry, and a longer prefix beats a shorter one; an asset that matches
// no entry defaults to session.
func assetAuth(assets []Asset, assetPath string) RouteAuth {
	auth := RouteAuthSession
	bestSpecificity := -1
	for _, asset := range assets {
		if !assetPathMatches(asset.Path, assetPath) {
			continue
		}
		specificity := assetSpecificity(asset.Path)
		if specificity > bestSpecificity {
			auth = asset.Auth
			bestSpecificity = specificity
		}
	}
	return auth
}

func assetSpecificity(assetPath string) int {
	if !strings.HasSuffix(assetPath, "*") {
		return 1 << 30
	}
	return len(strings.TrimSuffix(assetPath, "*"))
}

func assetPathMatches(pattern, assetPath string) bool {
	if pattern == assetPath {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(assetPath, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

// routePatternsOverlap reports whether two path patterns can match the same
// path. A trailing star matches a prefix. Shared by route and asset
// normalization so both reject ambiguous auth declarations the same way.
func routePatternsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	aStar := strings.HasSuffix(a, "*")
	bStar := strings.HasSuffix(b, "*")
	aPrefix := strings.TrimSuffix(a, "*")
	bPrefix := strings.TrimSuffix(b, "*")
	switch {
	case aStar && bStar:
		return strings.HasPrefix(aPrefix, bPrefix) || strings.HasPrefix(bPrefix, aPrefix)
	case aStar:
		return strings.HasPrefix(bPrefix, aPrefix)
	case bStar:
		return strings.HasPrefix(aPrefix, bPrefix)
	default:
		return false
	}
}
