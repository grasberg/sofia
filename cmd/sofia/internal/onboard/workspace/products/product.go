// Product configuration structure for digital products
package product

// Product represents a digital product with core and optional modules
type Product struct {
	Core     Core              `json:"core"              yaml:"core"`
	Modules  map[string]Module `json:"modules,omitempty" yaml:"modules,omitempty"`
	Metadata Metadata          `json:"metadata"          yaml:"metadata"`
}

// Core contains required product information
type Core struct {
	Name             string   `json:"name"                  yaml:"name"`
	ShortDescription string   `json:"short_description"     yaml:"short_description"`
	FullDescription  string   `json:"full_description"      yaml:"full_description"`
	Price            Price    `json:"price"                 yaml:"price"`
	Type             string   `json:"type"                  yaml:"type"` // digital_download, course, software, template
	DeliveryMethod   string   `json:"delivery_method"       yaml:"delivery_method"`
	License          string   `json:"license"               yaml:"license"` // personal, personal_commercial, commercial
	Files            []File   `json:"files"                 yaml:"files"`
	Category         string   `json:"category"              yaml:"category"`
	Subcategory      string   `json:"subcategory,omitempty" yaml:"subcategory,omitempty"`
	Tags             []string `json:"tags,omitempty"        yaml:"tags,omitempty"`
}

// Price configuration
type Price struct {
	Amount   float64 `json:"amount"          yaml:"amount"`
	Currency string  `json:"currency"        yaml:"currency"` // SEK, USD, EUR
	Tiers    []Tier  `json:"tiers,omitempty" yaml:"tiers,omitempty"`
}

// Tier for tiered pricing
type Tier struct {
	Name     string   `json:"name"               yaml:"name"`
	Amount   float64  `json:"amount"             yaml:"amount"`
	Features []string `json:"features,omitempty" yaml:"features,omitempty"`
}

// File represents a product file
type File struct {
	Name        string `json:"name"                  yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	SourcePath  string `json:"source_path"           yaml:"source_path"` // Relative path to source file
	OutputPath  string `json:"output_path"           yaml:"output_path"` // Generated file path
	Format      string `json:"format"                yaml:"format"`      // pdf, zip, mp4, etc.
	IsMain      bool   `json:"is_main,omitempty"     yaml:"is_main,omitempty"`
}

// Module represents an optional product module
type Module struct {
	Enabled  bool `json:"enabled"            yaml:"enabled"`
	Settings any  `json:"settings,omitempty" yaml:"settings,omitempty"`
}

// Predefined module settings types
type BonusMaterials struct {
	Materials []BonusMaterial `json:"materials" yaml:"materials"`
}

type BonusMaterial struct {
	Name        string `json:"name"        yaml:"name"`
	Description string `json:"description" yaml:"description"`
	FilePath    string `json:"file_path"   yaml:"file_path"`
}

type Campaigns struct {
	LaunchDiscount *LaunchDiscount `json:"launch_discount,omitempty" yaml:"launch_discount,omitempty"`
	CouponCodes    []CouponCode    `json:"coupon_codes,omitempty"    yaml:"coupon_codes,omitempty"`
}

type LaunchDiscount struct {
	Percentage   int `json:"percentage"    yaml:"percentage"`
	DurationDays int `json:"duration_days" yaml:"duration_days"`
}

type CouponCode struct {
	Code       string `json:"code"              yaml:"code"`
	Percentage int    `json:"percentage"        yaml:"percentage"`
	Expires    string `json:"expires,omitempty" yaml:"expires,omitempty"` // YYYY-MM-DD
}

type Upsells struct {
	Products []UpsellProduct `json:"products" yaml:"products"`
}

type UpsellProduct struct {
	ProductID string `json:"product_id" yaml:"product_id"`
	Name      string `json:"name"       yaml:"name"`
	Discount  int    `json:"discount"   yaml:"discount"` // percentage
}

type Affiliate struct {
	CommissionRate float64 `json:"commission_rate" yaml:"commission_rate"` // percentage
	CookieDuration int     `json:"cookie_duration" yaml:"cookie_duration"` // days
}

type EmailIntegration struct {
	Sequences []EmailSequence `json:"sequences" yaml:"sequences"`
}

type EmailSequence struct {
	Trigger   string `json:"trigger"    yaml:"trigger"` // purchase, download, etc.
	DaysAfter int    `json:"days_after" yaml:"days_after"`
	Subject   string `json:"subject"    yaml:"subject"`
	Template  string `json:"template"   yaml:"template"` // path to template file
}

type GumroadSettings struct {
	Visibility      string `json:"visibility"                 yaml:"visibility"`    // public, hidden
	PurchaseType    string `json:"purchase_type"              yaml:"purchase_type"` // fixed_price, pay_what_you_want
	Quantity        string `json:"quantity"                   yaml:"quantity"`      // unlimited, limited
	CustomPermalink string `json:"custom_permalink,omitempty" yaml:"custom_permalink,omitempty"`
	Thumbnail       string `json:"thumbnail,omitempty"        yaml:"thumbnail,omitempty"`
}

type BuildConfig struct {
	Scripts      []BuildScript `json:"scripts"                yaml:"scripts"`
	Dependencies []string      `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
}

type BuildScript struct {
	Name    string   `json:"name"              yaml:"name"`
	Command string   `json:"command"           yaml:"command"`
	Inputs  []string `json:"inputs,omitempty"  yaml:"inputs,omitempty"`
	Outputs []string `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}

type Testing struct {
	Tests []Test `json:"tests" yaml:"tests"`
}

type Test struct {
	Name   string `json:"name"   yaml:"name"`
	Script string `json:"script" yaml:"script"`
}

// Metadata about the product configuration
type Metadata struct {
	Version  string `json:"version"  yaml:"version"`
	Created  string `json:"created"  yaml:"created"`
	Updated  string `json:"updated"  yaml:"updated"`
	Author   string `json:"author"   yaml:"author"`
	Language string `json:"language" yaml:"language"` // sv, en, etc.
}
