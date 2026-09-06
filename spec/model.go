package spec

// Represent a complete framework-independent OpenAPI document.
// 表示完整 OpenAPI 文档；规范模型不依赖任何路由框架。
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
// 表示 API 的业务名称和版本信息。
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
// 表示维护联系信息。
type Contact struct {
	Name       string     `json:"name,omitempty"`
	URL        string     `json:"url,omitempty"`
	Email      string     `json:"email,omitempty"`
	Extensions Extensions `json:"-"`
}

// Describe a license identifier or URL.
// 表示许可证标识或地址。
type License struct {
	Name       string     `json:"name"`
	Identifier string     `json:"identifier,omitempty"`
	URL        string     `json:"url,omitempty"`
	Extensions Extensions `json:"-"`
}

// Describe server URLs and variable declarations.
// 表示服务地址及其变量声明。
type Server struct {
	URL         string                    `json:"url"`
	Description string                    `json:"description,omitempty"`
	Name        string                    `json:"name,omitempty"`
	Variables   map[string]ServerVariable `json:"variables,omitempty"`
	Extensions  Extensions                `json:"-"`
}

// Describe one server URL variable.
// 表示服务地址中的单个替换变量。
type ServerVariable struct {
	Enum        []string   `json:"enum,omitempty"`
	Default     string     `json:"default"`
	Description string     `json:"description,omitempty"`
	Extensions  Extensions `json:"-"`
}

// Group reusable components by kind, including OAS 3.2 media types.
// 保存按类别隔离的可复用组件，媒体类型也是三点二组件。
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
// 表示规范化路径，包含 QUERY 与扩展方法的独立入口。
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
// 表示单个接口契约，与源码模板标识相互独立。
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
	Deprecated   bool                            `json:"deprecated,omitempty"`
	Security     Optional[[]SecurityRequirement] `json:"security,omitzero"`
	Servers      []Server                        `json:"servers,omitempty"`
	Extensions   Extensions                      `json:"-"`
}

// Describe external documentation; renderers must encode it safely.
// 表示外部文档链接；渲染层负责安全编码。
type ExternalDocumentation struct {
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url"`
	Extensions  Extensions `json:"-"`
}

// Describe a named parameter or whole-querystring media contract.
// 表示命名参数或整个 querystring 的媒体类型契约。
type Parameter struct {
	Name            string                      `json:"name,omitempty"`
	In              string                      `json:"in"`
	Description     string                      `json:"description,omitempty"`
	Required        bool                        `json:"required,omitempty"`
	Deprecated      bool                        `json:"deprecated,omitempty"`
	AllowEmptyValue bool                        `json:"allowEmptyValue,omitempty"`
	Style           string                      `json:"style,omitempty"`
	Explode         Optional[bool]              `json:"explode,omitzero"`
	AllowReserved   bool                        `json:"allowReserved,omitempty"`
	Schema          *Schema                     `json:"schema,omitempty"`
	Example         Optional[any]               `json:"example,omitzero"`
	Examples        map[string]RefOr[Example]   `json:"examples,omitempty"`
	Content         map[string]RefOr[MediaType] `json:"content,omitempty"`
	Extensions      Extensions                  `json:"-"`
}

// Describe a request body whose required flag is separate from property requirements.
// 表示请求体；required 与内部属性 required 含义不同。
type RequestBody struct {
	Description string                      `json:"description,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content"`
	Required    bool                        `json:"required,omitempty"`
	Extensions  Extensions                  `json:"-"`
}

// Describe complete content, stream items, and multipart encodings.
// 表示单个媒体类型的完整数据、流式单项与 multipart 编码。
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
// 表示按名称或位置绑定的编码，支持嵌套 multipart。
type Encoding struct {
	ContentType    string                   `json:"contentType,omitempty"`
	Headers        map[string]RefOr[Header] `json:"headers,omitempty"`
	Style          string                   `json:"style,omitempty"`
	Explode        Optional[bool]           `json:"explode,omitzero"`
	AllowReserved  bool                     `json:"allowReserved,omitempty"`
	Encoding       map[string]Encoding      `json:"encoding,omitempty"`
	PrefixEncoding []Encoding               `json:"prefixEncoding,omitempty"`
	ItemEncoding   *Encoding                `json:"itemEncoding,omitempty"`
	Extensions     Extensions               `json:"-"`
}

// Describe response content, headers, and follow-up links.
// 表示响应的说明、头、媒体类型和后续链接。
type Response struct {
	// Native 3.2 response summary, independent of the detailed description.
	// 原生三点二响应摘要，与详细说明独立。
	Summary     string                      `json:"summary,omitempty"`
	Description string                      `json:"description,omitempty"`
	Headers     map[string]RefOr[Header]    `json:"headers,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content,omitempty"`
	Links       map[string]RefOr[Link]      `json:"links,omitempty"`
	Extensions  Extensions                  `json:"-"`
}

// Map callback expressions to path items.
// 表示回调表达式到路径项的映射。
type Callback map[string]RefOr[PathItem]

// Distinguish logical examples from serialized text and preserve explicit null.
// 区分逻辑示例值与真实序列化文本，显式 null 不省略。
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
// 表示链接参数和请求体表达式，不执行表达式中的业务操作。
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
// 表示响应头，名称由外层映射提供。
type Header struct {
	Description string                      `json:"description,omitempty"`
	Required    bool                        `json:"required,omitempty"`
	Deprecated  bool                        `json:"deprecated,omitempty"`
	Style       string                      `json:"style,omitempty"`
	Explode     Optional[bool]              `json:"explode,omitzero"`
	Schema      *Schema                     `json:"schema,omitempty"`
	Example     Optional[any]               `json:"example,omitzero"`
	Examples    map[string]RefOr[Example]   `json:"examples,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content,omitempty"`
	Extensions  Extensions                  `json:"-"`
}

// Describe OAS 3.2 tag hierarchy and classification.
// 表示三点二标签层级及分类。
type Tag struct {
	Name         string                 `json:"name"`
	Summary      string                 `json:"summary,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Parent       string                 `json:"parent,omitempty"`
	Kind         string                 `json:"kind,omitempty"`
	ExternalDocs *ExternalDocumentation `json:"externalDocs,omitempty"`
	Extensions   Extensions             `json:"-"`
}

// Describe security schemes with metadata URLs and deprecation flags.
// 表示安全方案，保留三点二的元数据地址和弃用标记。
type SecurityScheme struct {
	Type              string      `json:"type"`
	Description       string      `json:"description,omitempty"`
	Name              string      `json:"name,omitempty"`
	In                string      `json:"in,omitempty"`
	Scheme            string      `json:"scheme,omitempty"`
	BearerFormat      string      `json:"bearerFormat,omitempty"`
	Flows             *OAuthFlows `json:"flows,omitempty"`
	OpenIDConnectURL  string      `json:"openIdConnectUrl,omitempty"`
	OAuth2MetadataURL string      `json:"oauth2MetadataUrl,omitempty"`
	Deprecated        bool        `json:"deprecated,omitempty"`
	Extensions        Extensions  `json:"-"`
}

// Describe OAuth flows, including device authorization.
// 表示各类 OAuth 流，包括设备授权。
type OAuthFlows struct {
	Implicit            *OAuthFlow `json:"implicit,omitempty"`
	Password            *OAuthFlow `json:"password,omitempty"`
	ClientCredentials   *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode   *OAuthFlow `json:"authorizationCode,omitempty"`
	DeviceAuthorization *OAuthFlow `json:"deviceAuthorization,omitempty"`
	Extensions          Extensions `json:"-"`
}

// Describe authorization endpoints and required scope maps.
// 表示授权端点与必需的作用域映射。
type OAuthFlow struct {
	AuthorizationURL       string            `json:"authorizationUrl,omitempty"`
	DeviceAuthorizationURL string            `json:"deviceAuthorizationUrl,omitempty"`
	TokenURL               string            `json:"tokenUrl,omitempty"`
	RefreshURL             string            `json:"refreshUrl,omitempty"`
	Scopes                 map[string]string `json:"scopes"`
	Extensions             Extensions        `json:"-"`
}

// Combine schemes within each requirement and alternatives across the array.
// 同一映射内方案是共同要求，数组中的各映射是备选要求。
type SecurityRequirement map[string][]string
