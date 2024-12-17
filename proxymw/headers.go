package proxymw

type HeaderKey string

const (
	HeaderCriticality HeaderKey = "X-Request-Criticality"
	HeaderCanWait     HeaderKey = "X-Can-Wait"
)

var (
	HeaderDefaults = map[HeaderKey]string{
		HeaderCriticality: CriticalityDefault,
	}
)

func ParseHeaderKey(rr Request, key HeaderKey) string {
	vals := rr.Request().Header[string(key)]
	if len(vals) == 0 || vals[0] == "" {
		return HeaderDefaults[key]
	}
	return vals[0]
}
