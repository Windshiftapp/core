package objecttranslation

import "slices"

//go:generate node ../../scripts/generate-object-translation-catalog.mjs

// ShippedSystemTranslations returns the translations bundled with this build.
func ShippedSystemTranslations() []SystemTranslation {
	return slices.Clone(shippedSystemTranslations)
}
