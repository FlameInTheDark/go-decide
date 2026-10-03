// Package decide is a provider-agnostic Go client for System One style
// decision models: models that answer typed questions (choice, noul and
// score) about a piece of state and return calibrated probabilities instead
// of free-form text.
//
// # Domain model
//
// A [Request] carries a [State] (a string, an object or an array), one or
// more [Question]s and a model name. A [Provider] turns that request into a
// [Result] holding typed [Answer]s plus [Usage].
//
//	questions := []decide.Question{
//		decide.Choice("label", "Which label fits this ticket?", decide.Options{
//			"billing": "Payments and refunds",
//			"bug":     "Software errors",
//		}),
//		decide.Noul("is_urgent", "Does this block revenue?", decide.NoulCriteria{
//			False: "Revenue continues",
//			True:  "Revenue is blocked",
//		}),
//		decide.Score("urgency", "How urgent is this ticket?", decide.Scale{
//			"Can wait", "This week", "Now",
//		}),
//	}
//
//	req := decide.Request{
//		Model:     "nimble",
//		State:     decide.Text("Our checkout has returned 500 errors since 9am."),
//		Questions: questions,
//	}
//
//	res, err := client.Decide(ctx, req)
//
// # Providers
//
// Implement [Provider] to add a backend; nothing else in the package needs to
// change. The bundled adapters live in providers/ollama and
// providers/openrouter:
//
//	c := decide.New(
//		decide.WithProvider(ollama.New()),
//		decide.WithProvider(openrouter.New(openrouter.WithAPIKey(key))),
//		decide.WithDefault("ollama"),
//	)
//
//	res, err := c.DecideWith(ctx, "openrouter", req)
//
// [Question] marshals to the wire format shared by the System One endpoints,
// so adapters that speak that dialect can embed the type directly. A provider
// with a different dialect defines its own wire structs; see the provider
// packages for worked examples.
//
// # Capabilities
//
// A provider may implement [Capable] to advertise what it accepts. [Client]
// checks a request against those capabilities before sending it, so an
// unsupported image or an oversized state fails locally with a
// [KindUnsupported] or [KindPayloadTooLarge] error instead of a round trip.
package decide
