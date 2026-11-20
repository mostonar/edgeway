package sssmgr

type SssMgrBindingProtocol interface {
	init(tm *SssMgr, notify func(map[string]interface{})) (err error)
	send(p_ctx SssMgrBindingProtocolContext) (err error, resp interface{})
	uninit() (err error)
}

type SssMgrBindingProtocolContext struct {
	host    string
	port    int
	scheme  string
	path    string
	name    string
	hostId  string
	to      string
	from    string
	op      int
	ty      int
	rqi     string
	rvi     []string
	queries map[string]string
	body    map[string]map[string]interface{}
	code    int
}

/**
TS-0001 V4.22 Table 9.6.2.3-1: Types of parameters in accessControlOperations
TS-0004 V4.22 Table 6.3.4.2.5 1: Interpretation of m2m:operation
**/
