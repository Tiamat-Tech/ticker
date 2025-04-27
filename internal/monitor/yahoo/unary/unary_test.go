package unary_test

import (
	c "github.com/achannarasappa/ticker/v4/internal/common"
	"github.com/achannarasappa/ticker/v4/internal/monitor/yahoo/unary"
	. "github.com/onsi/ginkgo/v2"
	g "github.com/onsi/gomega/gstruct"

	"net/http"
	"net/url"

	. "github.com/onsi/gomega"

	"github.com/onsi/gomega/ghttp"
)

const (
	urlParams = "&fields=shortName,regularMarketChange,regularMarketChangePercent,regularMarketPrice,regularMarketPreviousClose,regularMarketOpen,regularMarketDayRange,regularMarketDayHigh,regularMarketDayLow,regularMarketVolume,postMarketChange,postMarketChangePercent,postMarketPrice,preMarketChange,preMarketChangePercent,preMarketPrice,fiftyTwoWeekHigh,fiftyTwoWeekLow,marketCap&formatted=true&lang=en-US&region=US&corsDomain=finance.yahoo.com"
)

var _ = Describe("Unary", func() {
	var (
		server *ghttp.Server
		client *unary.UnaryAPI
	)

	BeforeEach(func() {
		server = ghttp.NewServer()
		client = newTestClient(server)
	})

	AfterEach(func() {
		server.Close()
	})

	Describe("NewUnaryAPI", func() {
		It("should return a new UnaryAPI", func() {
			Expect(client).NotTo(BeNil())
		})
	})

	Describe("GetAssetQuotes", func() {
		It("should return a list of asset quotes", func() {
			appendQuoteHandler(server, "NET", responseQuote1Fixture)

			outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})

			Expect(outputSlice).To(g.MatchAllElementsWithIndex(g.IndexIdentity, g.Elements{
				"0": g.MatchFields(g.IgnoreExtras, g.Fields{
					"QuotePrice": g.MatchFields(g.IgnoreExtras, g.Fields{
						"Price":          Equal(84.98),
						"PricePrevClose": Equal(84.00),
						"PriceOpen":      Equal(85.22),
						"PriceDayHigh":   Equal(90.00),
						"PriceDayLow":    Equal(80.00),
						"Change":         Equal(3.0800018),
						"ChangePercent":  Equal(3.7606857),
					}),
					"QuoteSource": Equal(c.QuoteSourceYahoo),
					"Exchange": g.MatchFields(g.IgnoreExtras, g.Fields{
						"IsActive":                BeTrue(),
						"IsRegularTradingSession": BeTrue(),
					}),
					"Meta": g.MatchFields(g.IgnoreExtras, g.Fields{
						"SymbolInSourceAPI": Equal("NET"),
					}),
				}),
			}))
			Expect(outputMap).To(HaveKey("NET"))
			Expect(outputError).To(BeNil())
		})

		When("no symbols are provided", func() {
			It("should not return an error", func() {
				outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{})
				Expect(outputSlice).To(BeEmpty())
				Expect(outputMap).To(BeEmpty())
				Expect(outputError).To(BeNil())
			})
		})

		Context("when the request fails", func() {
			When("the response is invalid", func() {
				It("should return an error", func() {
					server.AppendHandlers(
						ghttp.CombineHandlers(
							ghttp.RespondWithJSONEncoded(http.StatusOK, "invalid"),
						),
					)

					outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
					Expect(outputSlice).To(BeEmpty())
					Expect(outputMap).To(BeEmpty())
					Expect(outputError).To(HaveOccurred())
				})
			})

			When("the request is invalid", func() {
				It("should return an error", func() {
					invalidClient := unary.NewUnaryAPI(unary.Config{
						BaseURL:           "invalid",
						SessionRootURL:    server.URL(),
						SessionCrumbURL:   server.URL(),
						SessionConsentURL: server.URL(),
					})

					outputSlice, outputMap, outputError := invalidClient.GetAssetQuotes([]string{"NET"})
					Expect(outputSlice).To(BeEmpty())
					Expect(outputMap).To(BeEmpty())
					Expect(outputError).To(HaveOccurred())
				})
			})
		})

		DescribeTable("market states",
			func(marketState string, postMarketPrice, preMarketPrice float64, expectedActive, expectedRegular bool, expectedPrice float64) {
				resp := cloneResponseQuote1Fixture()
				resp.QuoteResponse.Quotes[0].MarketState = marketState
				resp.QuoteResponse.Quotes[0].PostMarketPrice.Raw = postMarketPrice
				resp.QuoteResponse.Quotes[0].PreMarketPrice.Raw = preMarketPrice

				appendQuoteHandler(server, "NET", resp)

				outputSlice, _, outputError := client.GetAssetQuotes([]string{"NET"})
				Expect(outputSlice).To(g.MatchAllElementsWithIndex(g.IndexIdentity, g.Elements{
					"0": g.MatchFields(g.IgnoreExtras, g.Fields{
						"Exchange": g.MatchFields(g.IgnoreExtras, g.Fields{
							"IsActive":                Equal(expectedActive),
							"IsRegularTradingSession": Equal(expectedRegular),
						}),
						"QuotePrice": g.MatchFields(g.IgnoreExtras, g.Fields{
							"Price": Equal(expectedPrice),
						}),
					}),
				}))
				Expect(outputError).To(BeNil())
			},
			Entry("regular session", "REGULAR", 0.0, 0.0, true, true, 84.98),
			Entry("post market with price", "POST", 86.56, 0.0, true, false, 86.56),
			Entry("post market no price", "POST", 0.0, 0.0, true, false, 84.98),
			Entry("pre market with price", "PRE", 0.0, 84.98, true, false, 84.98),
			Entry("pre market no price", "PRE", 0.0, 0.0, false, false, 84.98),
			Entry("closed market", "CLOSED", 0.0, 0.0, false, false, 84.98),
			Entry("unknown with post market price", "UNKNOWN", 84.98, 0.0, false, false, 84.98),
		)

		It("should return the asset class as a cryptocurrency", func() {
			resp := cloneResponseQuote1Fixture()
			resp.QuoteResponse.Quotes[0].QuoteType = "CRYPTOCURRENCY"

			appendQuoteHandler(server, "NET", resp)

			outputSlice, _, outputError := client.GetAssetQuotes([]string{"NET"})
			Expect(outputSlice).To(g.MatchAllElementsWithIndex(g.IndexIdentity, g.Elements{
				"0": g.MatchFields(g.IgnoreExtras, g.Fields{
					"Class": Equal(c.AssetClassCryptocurrency),
				}),
			}))
			Expect(outputError).To(BeNil())
		})

		Context("session", func() {
			When("the session is not set or is expired", func() {
				It("should refresh the session and then retry the request", func() {
					// Setup all handlers using helper functions
					appendQuote401(server, "NET")
					appendRootSessionOK(server)
					appendCrumb(server, "abc123")
					appendQuoteWithCrumb(server, "NET", "abc123", responseQuote1Fixture)

					outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
					Expect(outputSlice).NotTo(BeEmpty())
					Expect(outputMap).To(HaveKey("NET"))
					Expect(outputError).To(BeNil())
				})

				When("the API redirects to the EU consent page", func() {
					It("should agree to the consent form and then retry the request", func() {
						// Setup all handlers using helper functions
						appendQuote401(server, "NET")
						appendConsentRedirect(server, "FPREfYw")
						appendConsentRedirect(server, "FPREfYw")
						appendConsentPrompt(server, "FPREfYw", "3_cc-session_f218784562897")
						appendConsentCollection(server, "3_cc-session_f218784562897", true)
						appendConsentSubmission(server, "3_cc-session_f218784562897", "FPREfYw",
							server.URL()+"/copyConsent?sessionId=3_cc-session_f218784562897&lang=de-DE")
						appendCopyConsent(server, "3_cc-session_f218784562897", true)
						appendCrumb(server, "gf34y383")
						appendQuoteWithCrumb(server, "NET", "gf34y383", responseQuote1Fixture)

						outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
						Expect(outputSlice).NotTo(BeEmpty())
						Expect(outputMap).To(HaveKey("NET"))
						Expect(outputError).To(BeNil())
					})

					When("the there is a problem agreeing to the consent form (HTTP protocol related error)", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							appendConsentRedirect(server, "FPREfYw")
							appendConsentRedirect(server, "FPREfYw")
							appendConsentPrompt(server, "FPREfYw", "3_cc-session_f218784562897")
							appendConsentCollection(server, "3_cc-session_f218784562897", true)

							// Use specific handler for bad location
							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("POST", "/v2/collectConsent", "sessionId=3_cc-session_f218784562897"),
									ghttp.VerifyForm(url.Values{
										"csrfToken": {"FPREfYw"},
										"sessionId": {"3_cc-session_f218784562897", "3_cc-session_f218784562897"},
										"namespace": {"yahoo"},
										"agree":     {"agree"},
									}),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Location", "://bad-url")
										w.WriteHeader(http.StatusFound)
									},
								),
							)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("error attempting to agree to EU consent request"))
							Expect(outputError.Error()).To(ContainSubstring("failed to parse Location header"))
						})
					})

					When("there is an issue creating the consent request", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							appendConsentRedirect(server, "FPREfYw")
							appendConsentRedirect(server, "FPREfYw")
							appendConsentPrompt(server, "FPREfYw", "3_cc-session_f218784562897")
							appendConsentCollection(server, "3_cc-session_f218784562897", true)
							appendConsentSubmission(server, "3_cc-session_f218784562897", "FPREfYw",
								server.URL()+"/copyConsent?sessionId=3_cc-session_f218784562897&lang=de-DE")
							appendCopyConsent(server, "3_cc-session_f218784562897", true)
							appendCrumb(server, "gf34y383")
							appendQuoteWithCrumb(server, "NET", "gf34y383", responseQuote1Fixture)

							// Use an invalid URL for consent endpoint
							invalidClient := unary.NewUnaryAPI(unary.Config{
								BaseURL:           server.URL(),
								SessionRootURL:    server.URL(),
								SessionCrumbURL:   server.URL(),
								SessionConsentURL: "http://\x7f.com",
							})

							outputSlice, outputMap, outputError := invalidClient.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("consent submission request"))
						})
					})

					When("the expected session cookie is not after agreeing to the consent form", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							appendConsentRedirect(server, "FPREfYw")
							appendConsentRedirect(server, "FPREfYw")
							appendConsentPrompt(server, "FPREfYw", "3_cc-session_f218784562897")
							appendConsentCollection(server, "3_cc-session_f218784562897", true)
							appendConsentSubmission(server, "3_cc-session_f218784562897", "FPREfYw",
								server.URL()+"/copyConsent?sessionId=3_cc-session_f218784562897&lang=de-DE")

							// CopyConsent without A3 cookie
							appendCopyConsent(server, "3_cc-session_f218784562897", false)
							appendCrumb(server, "gf34y383")
							appendQuoteWithCrumb(server, "NET", "gf34y383", responseQuote1Fixture)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("A3 session cookie missing"))
						})
					})

					When("the GUCS cookie is not set", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")

							// Both redirects without GUCS cookie
							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Location", server.URL()+"/consent?brandType=nonEu&gcrumb=FPREfYw&done=https://finance.yahoo.com/")
										w.WriteHeader(http.StatusTemporaryRedirect)
									},
								),
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Location", server.URL()+"/consent?brandType=nonEu&gcrumb=FPREfYw&done=https://finance.yahoo.com/")
										w.WriteHeader(http.StatusTemporaryRedirect)
									},
								),
							)

							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/consent", "brandType=nonEu&gcrumb=FPREfYw&done=https://finance.yahoo.com/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Location", server.URL()+"/v2/collectConsent?sessionId=3_cc-session_f218784562897")
										w.WriteHeader(http.StatusFound)
									},
								),
							)

							// Collection without GUCS cookie
							appendConsentCollection(server, "3_cc-session_f218784562897", false)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("GUCS cookie"))
						})
					})

					When("there is an issue extracting the session ID from the redirected request URL", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							appendConsentRedirect(server, "FPREfYw")
							appendConsentRedirect(server, "FPREfYw")

							// Consent prompt without sessionId in location
							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/consent", "brandType=nonEu&gcrumb=FPREfYw&done=https://finance.yahoo.com/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
										w.Header().Set("Location", server.URL()+"/v2/collectConsent")
										w.WriteHeader(http.StatusFound)
									},
								),
							)

							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/v2/collectConsent"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
										w.WriteHeader(http.StatusOK)
									},
								),
							)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("session id"))
						})
					})

					When("there is an issue extracting the CSRF token from the redirected request URL", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")

							// Missing gcrumb in redirection
							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
										w.Header().Set("Location", server.URL()+"/consent?brandType=nonEu&done=https://finance.yahoo.com/")
										w.WriteHeader(http.StatusTemporaryRedirect)
									},
								),
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
										w.Header().Set("Location", server.URL()+"/consent?brandType=nonEu&done=https://finance.yahoo.com/")
										w.WriteHeader(http.StatusTemporaryRedirect)
									},
								),
							)

							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/consent", "brandType=nonEu&done=https://finance.yahoo.com/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
										w.Header().Set("Location", server.URL()+"/v2/collectConsent?sessionId=3_cc-session_f218784562897")
										w.WriteHeader(http.StatusFound)
									},
								),
							)

							appendConsentCollection(server, "3_cc-session_f218784562897", true)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("CSRF token"))
						})
					})

					When("there is a problem refreshing the EU session (HTTP protocol related error)", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							appendConsentRedirect(server, "FPREfYw")
							appendMalformedURL(server)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("failed to parse Location header"))
						})
					})

					When("there is a problem getting the cookies (HTTP protocol related error)", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							appendMalformedURL(server)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("failed to parse Location header"))
						})
					})

					When("there is an issue getting the crumb", func() {
						When("there is an issue creating the crumb request", func() {
							It("should return an error", func() {
								appendQuote401(server, "NET")
								appendRootSessionOK(server)

								// Use an invalid URL for crumb endpoint
								invalidClient := unary.NewUnaryAPI(unary.Config{
									BaseURL:           server.URL(),
									SessionRootURL:    server.URL(),
									SessionCrumbURL:   "http://\x7f.com",
									SessionConsentURL: server.URL(),
								})

								outputSlice, outputMap, outputError := invalidClient.GetAssetQuotes([]string{"NET"})
								Expect(outputSlice).To(BeEmpty())
								Expect(outputMap).To(BeEmpty())
								Expect(outputError).To(HaveOccurred())
								Expect(outputError.Error()).To(ContainSubstring("crumb request"))
							})
						})

						When("the crumb request returns an unexpected HTTP status code", func() {
							It("should return an error", func() {
								appendQuote401(server, "NET")
								appendRootSessionOK(server)
								appendCrumbError(server, http.StatusInternalServerError)

								outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
								Expect(outputSlice).To(BeEmpty())
								Expect(outputMap).To(BeEmpty())
								Expect(outputError).To(HaveOccurred())
								Expect(outputError.Error()).To(ContainSubstring("crumb"))
							})
						})

						When("there is a HTTP protocol related error", func() {
							It("should return an error", func() {
								appendQuote401(server, "NET")
								appendRootSessionOK(server)
								server.AppendHandlers(
									ghttp.CombineHandlers(
										ghttp.VerifyRequest("GET", "/v1/test/getcrumb"),
										func(w http.ResponseWriter, r *http.Request) {
											w.Header().Set("Location", "://bad-url")
											w.WriteHeader(http.StatusFound)
										},
									),
								)

								outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
								Expect(outputSlice).To(BeEmpty())
								Expect(outputMap).To(BeEmpty())
								Expect(outputError).To(HaveOccurred())
								Expect(outputError.Error()).To(ContainSubstring("crumb"))
							})
						})

					})

					When("the expected session cookie is not set", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/"),
									func(w http.ResponseWriter, r *http.Request) {
										// No A3 cookie set
										w.WriteHeader(http.StatusOK)
									},
								),
							)
							appendCrumb(server, "abc123")
							appendQuoteWithCrumb(server, "NET", "abc123", responseQuote1Fixture)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("A3 session cookie missing"))
						})
					})

					When("the URL for the session root is invalid", func() {
						It("should return an error", func() {

							client := unary.NewUnaryAPI(unary.Config{
								BaseURL:           server.URL(),
								SessionRootURL:    "http://\x7f.com",
								SessionCrumbURL:   server.URL(),
								SessionConsentURL: server.URL(),
							})

							appendQuote401(server, "NET")
							appendRootSessionOK(server)
							appendCrumb(server, "abc123")
							appendQuoteWithCrumb(server, "NET", "abc123", responseQuote1Fixture)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("error creating cookie request"))
						})
					})

					When("the API returns an unexpected HTTP status code", func() {
						It("should return an error", func() {
							appendQuote401(server, "NET")
							appendConsentRedirect(server, "FPREfYw")
							server.AppendHandlers(
								ghttp.CombineHandlers(
									ghttp.VerifyRequest("GET", "/"),
									func(w http.ResponseWriter, r *http.Request) {
										w.WriteHeader(http.StatusInternalServerError)
									},
								),
							)

							outputSlice, outputMap, outputError := client.GetAssetQuotes([]string{"NET"})
							Expect(outputSlice).To(BeEmpty())
							Expect(outputMap).To(BeEmpty())
							Expect(outputError).To(HaveOccurred())
							Expect(outputError.Error()).To(ContainSubstring("non-2xx response code"))
						})
					})

				})
			})
		})
	})
})

// Create a new API client for testing
func newTestClient(server *ghttp.Server) *unary.UnaryAPI {
	return unary.NewUnaryAPI(unary.Config{
		BaseURL:           server.URL(),
		SessionRootURL:    server.URL(),
		SessionCrumbURL:   server.URL(),
		SessionConsentURL: server.URL(),
	})
}

// Clone the response fixture for modification in tests
func cloneResponseQuote1Fixture() unary.Response {
	// Deep copy the fixture if needed (depends on how responseQuote1Fixture is structured)
	return responseQuote1Fixture
}

// Basic quote handler for successful requests
func appendQuoteHandler(server *ghttp.Server, symbol string, response interface{}) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/v7/finance/quote", "symbols="+symbol+urlParams),
			ghttp.RespondWithJSONEncoded(http.StatusOK, response),
		),
	)
}

// Handler for 401 Unauthorized responses that trigger session refresh
func appendQuote401(server *ghttp.Server, symbol string) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/v7/finance/quote", "symbols="+symbol+urlParams),
			ghttp.RespondWith(http.StatusUnauthorized, ""),
		),
	)
}

// Quote handler with crumb parameter
func appendQuoteWithCrumb(server *ghttp.Server, symbol, crumb string, response interface{}) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/v7/finance/quote", "symbols="+symbol+urlParams+"&crumb="+crumb),
			ghttp.RespondWithJSONEncoded(http.StatusOK, response),
		),
	)
}

// Handler for successful session establishment
func appendRootSessionOK(server *ghttp.Server) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/"),
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Set-Cookie", "A3=d=AQABBA; Max-Age=31557600; path=/")
				w.WriteHeader(http.StatusOK)
			},
		),
	)
}

// Handler for crumb request
func appendCrumb(server *ghttp.Server, crumb string) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/v1/test/getcrumb"),
			ghttp.RespondWith(http.StatusOK, crumb),
		),
	)
}

// Handler for crumb request error
func appendCrumbError(server *ghttp.Server, statusCode int) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/v1/test/getcrumb"),
			ghttp.RespondWith(statusCode, ""),
		),
	)
}

// Handler for consent redirect
func appendConsentRedirect(server *ghttp.Server, gcrumb string) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/"),
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
				w.Header().Set("Location", server.URL()+"/consent?brandType=nonEu&gcrumb="+gcrumb+"&done=https://finance.yahoo.com/")
				w.WriteHeader(http.StatusTemporaryRedirect)
			},
		),
	)
}

// Handler for consent prompt
func appendConsentPrompt(server *ghttp.Server, gcrumb, sessionID string) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/consent", "brandType=nonEu&gcrumb="+gcrumb+"&done=https://finance.yahoo.com/"),
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
				w.Header().Set("Location", server.URL()+"/v2/collectConsent?sessionId="+sessionID)
				w.WriteHeader(http.StatusFound)
			},
		),
	)
}

// Handler for consent collection
func appendConsentCollection(server *ghttp.Server, sessionID string, setCookie bool) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/v2/collectConsent", "sessionId="+sessionID),
			func(w http.ResponseWriter, r *http.Request) {
				if setCookie {
					w.Header().Set("Set-Cookie", "GUCS=test1234; path=/")
				}
				w.WriteHeader(http.StatusOK)
			},
		),
	)
}

// Handler for consent submission
func appendConsentSubmission(server *ghttp.Server, sessionID, gcrumb, location string) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("POST", "/v2/collectConsent", "sessionId="+sessionID),
			ghttp.VerifyForm(url.Values{
				"csrfToken": {gcrumb},
				"sessionId": {sessionID, sessionID},
				"namespace": {"yahoo"},
				"agree":     {"agree"},
			}),
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusFound)
			},
		),
	)
}

// Handler for copy consent
func appendCopyConsent(server *ghttp.Server, sessionID string, setA3Cookie bool) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/copyConsent", "sessionId="+sessionID+"&lang=de-DE"),
			func(w http.ResponseWriter, r *http.Request) {
				if setA3Cookie {
					w.Header().Set("Set-Cookie", "A3=d=AQABKKGUo4YcCE; Path=/")
				}
				w.Header().Set("Location", server.URL()+"/?guccounter=1")
				w.WriteHeader(http.StatusFound)
			},
		),
	)
}

// Handler for malformed URL
func appendMalformedURL(server *ghttp.Server) {
	server.AppendHandlers(
		ghttp.CombineHandlers(
			ghttp.VerifyRequest("GET", "/"),
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "://bad-url")
				w.WriteHeader(http.StatusFound)
			},
		),
	)
}
