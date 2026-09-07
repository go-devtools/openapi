package spec

// Represent a complete framework-independent OpenAPI document.
type OpenAPI struct {
	OpenAPI           string                          `json:"openapi"`
	Self              string                          `json:"$self,omitempty"`
	Info              Info                            `json:"info"`
	JSONSchemaDialect string                          `json:"jsonSchemaDialect,omitempty"`
	Servers           []Server                        `json:"servers,omitempty"`
	Paths             map[string]*PathItem            `json:"paths,omitempty"`
	Webhooks          map[string]RefOr[PathItem]      `json:"webhooks,omitempty"`
	Components        *Components                     `json:"components,omitempty"`
	Security          Optional[[]SecurityRequirement] `json:"security,omitzero"`
	Tags              []Tag                           `json:"tags,omitempty"`
	ExternalDocs      *ExternalDocumentation          `json:"externalDocs,omitempty"`
	Extensions        Extensions                      `json:"-"`
}

// Describe the API title and version.
type Info struct {
	Title          string     `json:"title"`
	Summary        string     `json:"summary,omitempty"`
	Description    string     `json:"description,omitempty"`
	TermsOfService string     `json:"termsOfService,omitempty"`
	Contact        *Contact   `json:"contact,omitempty"`
	License        *License   `json:"license,omitempty"`
	Version        string     `json:"version"`
	Extensions     Extensions `json:"-"`
}

// Describe maintenance contact information.
type Contact struct {
	Name       string     `json:"name,omitempty"`
	URL        string     `json:"url,omitempty"`
	Email      string     `json:"email,omitempty"`
	Extensions Extensions `json:"-"`
}

// Describe a license identifier or URL.
type License struct {
	Name       string     `json:"name"`
	Identifier string     `json:"identifier,omitempty"`
	URL        string     `json:"url,omitempty"`
	Extensions Extensions `json:"-"`
}

// Describe server URLs and variable declarations.
type Server struct {
	URL         string                    `json:"url"`
	Description string                    `json:"description,omitempty"`
	Name        string                    `json:"name,omitempty"`
	Variables   map[string]ServerVariable `json:"variables,omitempty"`
	Extensions  Extensions                `json:"-"`
}

// Describe one server URL variable.
type ServerVariable struct {
	Enum        []string   `json:"enum,omitempty"`
	Default     string     `json:"default"`
	Description string     `json:"description,omitempty"`
	Extensions  Extensions `json:"-"`
}

// Group reusable components by kind, including OAS 3.2 media types.
type Components struct {
	Schemas         map[string]*Schema               `json:"schemas,omitempty"`
	Responses       map[string]RefOr[Response]       `json:"responses,omitempty"`
	Parameters      map[string]RefOr[Parameter]      `json:"parameters,omitempty"`
	Examples        map[string]RefOr[Example]        `json:"examples,omitempty"`
	RequestBodies   map[string]RefOr[RequestBody]    `json:"requestBodies,omitempty"`
	Headers         map[string]RefOr[Header]         `json:"headers,omitempty"`
	SecuritySchemes map[string]RefOr[SecurityScheme] `json:"securitySchemes,omitempty"`
	Links           map[string]RefOr[Link]           `json:"links,omitempty"`
	Callbacks       map[string]RefOr[Callback]       `json:"callbacks,omitempty"`
	PathItems       map[string]RefOr[PathItem]       `json:"pathItems,omitempty"`
	MediaTypes      map[string]RefOr[MediaType]      `json:"mediaTypes,omitempty"`
	Extensions      Extensions                       `json:"-"`
}

// Represent normalized paths with separate QUERY and additional operation fields.
type PathItem struct {
	Ref                  string                `json:"$ref,omitempty"`
	Summary              string                `json:"summary,omitempty"`
	Description          string                `json:"description,omitempty"`
	Get                  *Operation            `json:"get,omitempty"`
	Put                  *Operation            `json:"put,omitempty"`
	Post                 *Operation            `json:"post,omitempty"`
	Delete               *Operation            `json:"delete,omitempty"`
	Options              *Operation            `json:"options,omitempty"`
	Head                 *Operation            `json:"head,omitempty"`
	Patch                *Operation            `json:"patch,omitempty"`
	Trace                *Operation            `json:"trace,omitempty"`
	Query                *Operation            `json:"query,omitempty"`
	AdditionalOperations map[string]*Operation `json:"additionalOperations,omitempty"`
	Servers              []Server              `json:"servers,omitempty"`
	Parameters           []RefOr[Parameter]    `json:"parameters,omitempty"`
	Extensions           Extensions            `json:"-"`
}

// Represent an operation independently of source template identities.
type Operation struct {
	Tags         []string                        `json:"tags,omitempty"`
	Summary      string                          `json:"summary,omitempty"`
	Description  string                          `json:"description,omitempty"`
	ExternalDocs *ExternalDocumentation          `json:"externalDocs,omitempty"`
	OperationID  string                          `json:"operationId,omitempty"`
	Parameters   []RefOr[Parameter]              `json:"parameters,omitempty"`
	RequestBody  *RefOr[RequestBody]             `json:"requestBody,omitempty"`
	Responses    map[string]RefOr[Response]      `json:"responses,omitempty"`
	Callbacks    map[string]RefOr[Callback]      `json:"callbacks,omitempty"`
	Deprecated   Optional[bool]                  `json:"deprecated,omitzero"`
	Security     Optional[[]SecurityRequirement] `json:"security,omitzero"`
	Servers      []Server                        `json:"servers,omitempty"`
	Extensions   Extensions                      `json:"-"`
}

// Describe external documentation; renderers must encode it safely.
type ExternalDocumentation struct {
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url"`
	Extensions  Extensions `json:"-"`
}

// Describe a named parameter or whole-querystring media contract.
type Parameter struct {
	Name            string                      `json:"name"`
	In              string                      `json:"in"`
	Description     string                      `json:"description,omitempty"`
	Required        Optional[bool]              `json:"required,omitzero"`
	Deprecated      Optional[bool]              `json:"deprecated,omitzero"`
	AllowEmptyValue Optional[bool]              `json:"allowEmptyValue,omitzero"`
	Style           string                      `json:"style,omitempty"`
	Explode         Optional[bool]              `json:"explode,omitzero"`
	AllowReserved   Optional[bool]              `json:"allowReserved,omitzero"`
	Schema          *Schema                     `json:"schema,omitempty"`
	Example         Optional[any]               `json:"example,omitzero"`
	Examples        map[string]RefOr[Example]   `json:"examples,omitempty"`
	Content         map[string]RefOr[MediaType] `json:"content,omitempty"`
	Extensions      Extensions                  `json:"-"`
}

// Describe a request body whose required flag is separate from property requirements.
type RequestBody struct {
	Description string                      `json:"description,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content"`
	Required    Optional[bool]              `json:"required,omitzero"`
	Extensions  Extensions                  `json:"-"`
}

// Describe complete content, stream items, and multipart encodings.
type MediaType struct {
	Description    string                    `json:"description,omitempty"`
	Schema         *Schema                   `json:"schema,omitempty"`
	ItemSchema     *Schema                   `json:"itemSchema,omitempty"`
	Example        Optional[any]             `json:"example,omitzero"`
	Examples       map[string]RefOr[Example] `json:"examples,omitempty"`
	Encoding       map[string]Encoding       `json:"encoding,omitempty"`
	PrefixEncoding []Encoding                `json:"prefixEncoding,omitempty"`
	ItemEncoding   *Encoding                 `json:"itemEncoding,omitempty"`
	Extensions     Extensions                `json:"-"`
}

// Describe named or positional encodings, including nested multipart content.
type Encoding struct {
	ContentType    string                   `json:"contentType,omitempty"`
	Headers        map[string]RefOr[Header] `json:"headers,omitempty"`
	Style          string                   `json:"style,omitempty"`
	Explode        Optional[bool]           `json:"explode,omitzero"`
	AllowReserved  Optional[bool]           `json:"allowReserved,omitzero"`
	Encoding       map[string]Encoding      `json:"encoding,omitempty"`
	PrefixEncoding []Encoding               `json:"prefixEncoding,omitempty"`
	ItemEncoding   *Encoding                `json:"itemEncoding,omitempty"`
	Extensions     Extensions               `json:"-"`
}

// Describe response content, headers, and follow-up links.
type Response struct {
	// Native 3.2 response summary, independent of the detailed description.
	Summary     string                      `json:"summary,omitempty"`
	Description string                      `json:"description,omitempty"`
	Headers     map[string]RefOr[Header]    `json:"headers,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content,omitempty"`
	Links       map[string]RefOr[Link]      `json:"links,omitempty"`
	Extensions  Extensions                  `json:"-"`
}

// Map callback expressions to path items.
type Callback map[string]RefOr[PathItem]

// Distinguish logical examples from serialized text and preserve explicit null.
type Example struct {
	Summary         string           `json:"summary,omitempty"`
	Description     string           `json:"description,omitempty"`
	Value           Optional[any]    `json:"value,omitzero"`
	DataValue       Optional[any]    `json:"dataValue,omitzero"`
	SerializedValue Optional[string] `json:"serializedValue,omitzero"`
	ExternalValue   string           `json:"externalValue,omitempty"`
	Extensions      Extensions       `json:"-"`
}

// Describe link expressions without executing business operations.
type Link struct {
	OperationRef string         `json:"operationRef,omitempty"`
	OperationID  string         `json:"operationId,omitempty"`
	Parameters   map[string]any `json:"parameters,omitempty"`
	RequestBody  Optional[any]  `json:"requestBody,omitzero"`
	Description  string         `json:"description,omitempty"`
	Server       *Server        `json:"server,omitempty"`
	Extensions   Extensions     `json:"-"`
}

// Describe a response header named by its containing map.
type Header struct {
	Description string                      `json:"description,omitempty"`
	Required    Optional[bool]              `json:"required,omitzero"`
	Deprecated  Optional[bool]              `json:"deprecated,omitzero"`
	Style       string                      `json:"style,omitempty"`
	Explode     Optional[bool]              `json:"explode,omitzero"`
	Schema      *Schema                     `json:"schema,omitempty"`
	Example     Optional[any]               `json:"example,omitzero"`
	Examples    map[string]RefOr[Example]   `json:"examples,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content,omitempty"`
	Extensions  Extensions                  `json:"-"`
}

// Describe OAS 3.2 tag hierarchy and classification.
type Tag struct {
	Name        string `json:"name"`
	Summary     string `json:"summary,omitempty"`
	Description string `json:"description,omitempty"`
	// Preserve an explicit empty parent, which refers to an empty tag name.
	Parent       Optional[string]       `json:"parent,omitzero"`
	Kind         string                 `json:"kind,omitempty"`
	ExternalDocs *ExternalDocumentation `json:"externalDocs,omitempty"`
	Extensions   Extensions             `json:"-"`
}

// Describe security schemes with metadata URLs and deprecation flags.
type SecurityScheme struct {
	Type              string         `json:"type"`
	Description       string         `json:"description,omitempty"`
	Name              string         `json:"name,omitempty"`
	In                string         `json:"in,omitempty"`
	Scheme            string         `json:"scheme,omitempty"`
	BearerFormat      string         `json:"bearerFormat,omitempty"`
	Flows             *OAuthFlows    `json:"flows,omitempty"`
	OpenIDConnectURL  string         `json:"openIdConnectUrl,omitempty"`
	OAuth2MetadataURL string         `json:"oauth2MetadataUrl,omitempty"`
	Deprecated        Optional[bool] `json:"deprecated,omitzero"`
	Extensions        Extensions     `json:"-"`
}

// Describe OAuth flows, including device authorization.
type OAuthFlows struct {
	Implicit            *OAuthFlow `json:"implicit,omitempty"`
	Password            *OAuthFlow `json:"password,omitempty"`
	ClientCredentials   *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode   *OAuthFlow `json:"authorizationCode,omitempty"`
	DeviceAuthorization *OAuthFlow `json:"deviceAuthorization,omitempty"`
	Extensions          Extensions `json:"-"`
}

// Describe authorization endpoints and required scope maps.
type OAuthFlow struct {
	AuthorizationURL       string            `json:"authorizationUrl,omitempty"`
	DeviceAuthorizationURL string            `json:"deviceAuthorizationUrl,omitempty"`
	TokenURL               string            `json:"tokenUrl,omitempty"`
	RefreshURL             string            `json:"refreshUrl,omitempty"`
	Scopes                 map[string]string `json:"scopes"`
	Extensions             Extensions        `json:"-"`
}

// Combine schemes within each requirement and alternatives across the array.
type SecurityRequirement map[string][]string
