package spec

// 表示完整 OpenAPI 文档；规范模型不依赖任何路由框架。
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

// 表示 API 的业务名称和版本信息。
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

// 表示维护联系信息。
// Describe maintenance contact information.
type Contact struct {
	Name       string     `json:"name,omitempty"`
	URL        string     `json:"url,omitempty"`
	Email      string     `json:"email,omitempty"`
	Extensions Extensions `json:"-"`
}

// 表示许可证标识或地址。
// Describe a license identifier or URL.
type License struct {
	Name       string     `json:"name"`
	Identifier string     `json:"identifier,omitempty"`
	URL        string     `json:"url,omitempty"`
	Extensions Extensions `json:"-"`
}

// 表示服务地址及其变量声明。
// Describe server URLs and variable declarations.
type Server struct {
	URL         string                    `json:"url"`
	Description string                    `json:"description,omitempty"`
	Name        string                    `json:"name,omitempty"`
	Variables   map[string]ServerVariable `json:"variables,omitempty"`
	Extensions  Extensions                `json:"-"`
}

// 表示服务地址中的单个替换变量。
// Describe one server URL variable.
type ServerVariable struct {
	Enum        []string   `json:"enum,omitempty"`
	Default     string     `json:"default"`
	Description string     `json:"description,omitempty"`
	Extensions  Extensions `json:"-"`
}

// 保存按类别隔离的可复用组件，媒体类型也是三点二组件。
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

// 表示规范化路径，包含 QUERY 与扩展方法的独立入口。
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

// 表示单个接口契约，与源码模板标识相互独立。
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
	Deprecated   bool                            `json:"deprecated,omitempty"`
	Security     Optional[[]SecurityRequirement] `json:"security,omitzero"`
	Servers      []Server                        `json:"servers,omitempty"`
	Extensions   Extensions                      `json:"-"`
}

// 表示外部文档链接；渲染层负责安全编码。
// Describe external documentation; renderers must encode it safely.
type ExternalDocumentation struct {
	Description string     `json:"description,omitempty"`
	URL         string     `json:"url"`
	Extensions  Extensions `json:"-"`
}

// 表示命名参数或整个 querystring 的媒体类型契约。
// Describe a named parameter or whole-querystring media contract.
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

// 表示请求体；required 与内部属性 required 含义不同。
// Describe a request body whose required flag is separate from property requirements.
type RequestBody struct {
	Description string                      `json:"description,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content"`
	Required    bool                        `json:"required,omitempty"`
	Extensions  Extensions                  `json:"-"`
}

// 表示单个媒体类型的完整数据、流式单项与 multipart 编码。
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

// 表示按名称或位置绑定的编码，支持嵌套 multipart。
// Describe named or positional encodings, including nested multipart content.
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

// 表示响应的说明、头、媒体类型和后续链接。
// Describe response content, headers, and follow-up links.
type Response struct {
	// 原生三点二响应摘要，与详细说明独立。
	// Native 3.2 response summary, independent of the detailed description.
	Summary     string                      `json:"summary,omitempty"`
	Description string                      `json:"description,omitempty"`
	Headers     map[string]RefOr[Header]    `json:"headers,omitempty"`
	Content     map[string]RefOr[MediaType] `json:"content,omitempty"`
	Links       map[string]RefOr[Link]      `json:"links,omitempty"`
	Extensions  Extensions                  `json:"-"`
}

// 表示回调表达式到路径项的映射。
// Map callback expressions to path items.
type Callback map[string]RefOr[PathItem]

// 区分逻辑示例值与真实序列化文本，显式 null 不省略。
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

// 表示链接参数和请求体表达式，不执行表达式中的业务操作。
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

// 表示响应头，名称由外层映射提供。
// Describe a response header named by its containing map.
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

// 表示三点二标签层级及分类。
// Describe OAS 3.2 tag hierarchy and classification.
type Tag struct {
	Name         string                 `json:"name"`
	Summary      string                 `json:"summary,omitempty"`
	Description  string                 `json:"description,omitempty"`
	Parent       string                 `json:"parent,omitempty"`
	Kind         string                 `json:"kind,omitempty"`
	ExternalDocs *ExternalDocumentation `json:"externalDocs,omitempty"`
	Extensions   Extensions             `json:"-"`
}

// 表示安全方案，保留三点二的元数据地址和弃用标记。
// Describe security schemes with metadata URLs and deprecation flags.
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

// 表示各类 OAuth 流，包括设备授权。
// Describe OAuth flows, including device authorization.
type OAuthFlows struct {
	Implicit            *OAuthFlow `json:"implicit,omitempty"`
	Password            *OAuthFlow `json:"password,omitempty"`
	ClientCredentials   *OAuthFlow `json:"clientCredentials,omitempty"`
	AuthorizationCode   *OAuthFlow `json:"authorizationCode,omitempty"`
	DeviceAuthorization *OAuthFlow `json:"deviceAuthorization,omitempty"`
	Extensions          Extensions `json:"-"`
}

// 表示授权端点与必需的作用域映射。
// Describe authorization endpoints and required scope maps.
type OAuthFlow struct {
	AuthorizationURL       string            `json:"authorizationUrl,omitempty"`
	DeviceAuthorizationURL string            `json:"deviceAuthorizationUrl,omitempty"`
	TokenURL               string            `json:"tokenUrl,omitempty"`
	RefreshURL             string            `json:"refreshUrl,omitempty"`
	Scopes                 map[string]string `json:"scopes"`
	Extensions             Extensions        `json:"-"`
}

// 同一映射内方案是共同要求，数组中的各映射是备选要求。
// Combine schemes within each requirement and alternatives across the array.
type SecurityRequirement map[string][]string
