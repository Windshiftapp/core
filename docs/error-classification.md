# Error classification contract

How service-layer failures become HTTP statuses in the v2 API.

## Rule

Classify errors by type at the producer; never by message text at the
consumer. A transport mapper (`itemError`, `linkError`, `actionError`, …)
must decide between 4xx and 5xx using `errors.Is` / `errors.As` only.

Message sniffing (`strings.Contains(err.Error(), …)`) is banned in
`internal/restapi/v2` and in service-layer classifiers. It misclassifies
internal failures whose text happens to contain a sniffed phrase ("…is
required", "not allowed") as client errors, silently hides real 500s, and
couples the wire contract to error wording.

## Patterns

- Distinct, enumerable conditions use sentinel errors plus `errors.Is`
  (`services.ErrLinkExists`, `repository.ErrNotFound`).
- Open-ended client mistakes use `services.InvalidRequestError` plus
  `errors.As`. The producer sets the exact user-facing message; the mapper
  renders it as `400 invalid_request` and adds field details when it knows
  the field.
- Structured, multi-facet validation keeps its own type
  (`services.ActionValidationError`, `validation.ValidationError`,
  `services.TransitionRejection`).

Wire compatibility note: `InvalidRequestError` replaced the former
substring sniffing in `linkError` and `actionError` with byte-identical
messages for every previously matched error, and correctly reclassifies
two formerly-500 client mistakes (`primary field not found`,
`field is not a linking type`) as 400.

## Consumers

Regression tests live in the sibling `core-tests` repository:

- `internal/restapi/v2/error_mapping_test.go`: mappers classify by type;
  untyped errors containing sniffed phrases stay internal errors.
- `internal/services/actioncatalog/validate_config_test.go`: config parse
  vs shape classification through the error chain.

## Outstanding legacy sites

The v1 handler layer still matches error strings by equality
(`internal/handlers/scim.go` "http: request body too large" sites should
use `errors.As(&http.MaxBytesError{})`; `scim_tokens.go:114`,
`attachment_settings.go:61`, `api_tokens.go:313`, `channels.go:323`).
These predate v2 and are tracked for a dedicated sweep; do not add new
ones.
