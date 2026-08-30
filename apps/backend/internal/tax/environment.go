package tax

import "github.com/google/uuid"

// The read-only view conditions and effects receive at eval time - intentionally narrow, and later stages widen it as new condition types need more facts
type Environment interface {
	ChannelID() uuid.UUID
	CountryCode() string
	RegionCode() string
	// The amount tax is computed against, typically the post-discount subtotal
	TaxableBaseCents() int64
	TaxInclusive() bool
	// The tax class of the line being taxed in per-line evaluation - empty in whole-order evaluation
	ProductTaxClass() string
	// The placing customer's group - the customer tax dimension, for exemptions and reverse-charge
	CustomerGroupSlug() string
}

type EnvironmentInput struct {
	ChannelID         uuid.UUID
	CountryCode       string
	RegionCode        string
	TaxableCents      int64
	TaxInclusive      bool
	ProductTaxClass   string
	CustomerGroupSlug string
}

type envImpl struct {
	channelID     uuid.UUID
	country       string
	region        string
	taxableCents  int64
	inclusive     bool
	taxClass      string
	customerGroup string
}

func BuildEnvironment(in EnvironmentInput) Environment {
	return &envImpl{
		channelID:     in.ChannelID,
		country:       in.CountryCode,
		region:        in.RegionCode,
		taxableCents:  in.TaxableCents,
		inclusive:     in.TaxInclusive,
		taxClass:      in.ProductTaxClass,
		customerGroup: in.CustomerGroupSlug,
	}
}

func (e *envImpl) ChannelID() uuid.UUID      { return e.channelID }
func (e *envImpl) CountryCode() string       { return e.country }
func (e *envImpl) RegionCode() string        { return e.region }
func (e *envImpl) TaxableBaseCents() int64   { return e.taxableCents }
func (e *envImpl) TaxInclusive() bool        { return e.inclusive }
func (e *envImpl) ProductTaxClass() string   { return e.taxClass }
func (e *envImpl) CustomerGroupSlug() string { return e.customerGroup }
