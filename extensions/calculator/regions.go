package calculator

import "strings"

// regionCurrency is the currency of a country, by its ISO 3166 code, as BRL of "BR"; USD when
// unknown.
func regionCurrency(region string) string {
	region = strings.ToUpper(region)
	for _, euro := range strings.Fields(`AD AT BE CY DE EE ES FI FR GR HR IE IT LT LU LV MC ME MT NL PT
		SI SK SM VA XK`) {
		if region == euro {
			return "EUR"
		}
	}
	if c, ok := currencies[region]; ok {
		return c
	}
	return "USD"
}

// currencies are the countries' own currencies, beyond the euro's.
var currencies = func() map[string]string {
	m := map[string]string{}
	for _, pair := range strings.Fields(`AE:AED AF:AFN AL:ALL AM:AMD AO:AOA AR:ARS AU:AUD AZ:AZN BA:BAM
		BB:BBD BD:BDT BG:BGN BH:BHD BI:BIF BN:BND BO:BOB BR:BRL BS:BSD BT:BTN BW:BWP BY:BYN BZ:BZD
		CA:CAD CD:CDF CH:CHF CL:CLP CN:CNY CO:COP CR:CRC CU:CUP CV:CVE CZ:CZK DJ:DJF DK:DKK DO:DOP
		DZ:DZD EG:EGP ER:ERN ET:ETB FJ:FJD GB:GBP GE:GEL GH:GHS GM:GMD GN:GNF GT:GTQ GY:GYD HK:HKD
		HN:HNL HT:HTG HU:HUF ID:IDR IL:ILS IN:INR IQ:IQD IR:IRR IS:ISK JM:JMD JO:JOD JP:JPY KE:KES
		KG:KGS KH:KHR KR:KRW KW:KWD KZ:KZT LA:LAK LB:LBP LI:CHF LK:LKR LR:LRD LS:LSL LY:LYD MA:MAD
		MD:MDL MG:MGA MK:MKD MM:MMK MN:MNT MO:MOP MR:MRU MU:MUR MV:MVR MW:MWK MX:MXN MY:MYR MZ:MZN
		NA:NAD NG:NGN NI:NIO NO:NOK NP:NPR NZ:NZD OM:OMR PE:PEN PG:PGK PH:PHP PK:PKR PL:PLN PY:PYG
		QA:QAR RO:RON RS:RSD RU:RUB RW:RWF SA:SAR SC:SCR SD:SDG SE:SEK SG:SGD SL:SLE SO:SOS SR:SRD
		SS:SSP ST:STN SY:SYP SZ:SZL TH:THB TJ:TJS TM:TMT TN:TND TO:TOP TR:TRY TT:TTD TW:TWD TZ:TZS
		UA:UAH UG:UGX UY:UYU UZ:UZS VE:VES VN:VND VU:VUV WS:WST YE:YER ZA:ZAR ZM:ZMW
		CM:XAF CF:XAF TD:XAF CG:XAF GQ:XAF GA:XAF BJ:XOF BF:XOF CI:XOF GW:XOF ML:XOF NE:XOF SN:XOF
		TG:XOF PF:XPF NC:XPF`) {
		country, code, _ := strings.Cut(pair, ":")
		m[country] = code
	}
	return m
}()
