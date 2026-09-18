package objecttranslation

import "slices"

//go:generate bun ../../scripts/generate-object-translation-catalog.mjs

// ShippedSystemTranslations returns the translations bundled with this build.
func ShippedSystemTranslations() []SystemTranslation {
	return append(slices.Clone(shippedSystemTranslations), shippedLinkLabelTranslations...)
}
