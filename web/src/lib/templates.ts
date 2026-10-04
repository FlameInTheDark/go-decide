import type { DecideRequest } from "./types"

export interface Template {
  id: string
  label: string
  summary: string
  state: unknown
  questions: DecideRequest["questions"]
}

export const templates: Template[] = [
  {
    id: "blank",
    label: "Blank",
    summary: "One empty choice question to start from",
    state: "Some text to decide about.",
    questions: [
      { name: "category", type: "choice", instructions: "Which one is this?", options_list: [{ key: "a", text: "First" }, { key: "b", text: "Second" }] },
    ],
  },
  {
    id: "support-routing",
    label: "Support routing",
    summary: "Route a ticket, spot a refund, score the urgency",
    state: {
      ticket: "Checkout returns HTTP 500 for every card payment since 14:02.",
      customer_tier: "enterprise",
      previous_contacts: 2,
    },
    questions: [
      {
        name: "category",
        type: "choice",
        instructions: "Which team owns this ticket?",
        options_list: [
          { key: "payments", text: "Payments, invoices or refunds" },
          { key: "product", text: "The product is broken or unavailable" },
          { key: "account", text: "Login, permissions or profile" },
        ],
      },
      {
        name: "refund",
        type: "noul",
        instructions: "Is the customer explicitly asking for money back?",
        outcomes: { false: "No refund is requested", true: "A refund is explicitly requested" },
      },
      {
        name: "urgency",
        type: "score",
        instructions: "How urgent is this ticket?",
        scale: ["Can wait for the next release", "Should be fixed this week", "Blocking revenue right now"],
      },
    ],
  },
  {
    id: "intent-routing",
    label: "Intent routing",
    summary: "Classify a support message, with other as a safety net",
    state: "how do I rotate my API key",
    questions: [
      {
        name: "intent",
        type: "choice",
        instructions: "What is the user trying to do?",
        options_list: [
          { key: "billing", text: "Charges, invoices, refunds or plans" },
          { key: "technical", text: "An error, an outage or something not working" },
          { key: "account", text: "Login, password, permissions or profile" },
          { key: "how_to", text: "Step by step help using a feature" },
          { key: "sales", text: "Pricing, purchasing or a sales question" },
          { key: "feedback", text: "Praise, complaints or a feature request" },
          { key: "other", text: "None of the above" },
        ],
      },
    ],
  },
  {
    id: "content-moderation",
    label: "Content moderation",
    summary: "Four policy checks asked in one request",
    state: "buy cheap followers now, limited time offer",
    questions: [
      { name: "threats", type: "noul", instructions: "Does this text contain a threat of violence?", outcomes: { false: "No", true: "Yes" } },
      { name: "self_harm", type: "noul", instructions: "Does this text describe or encourage self-harm?", outcomes: { false: "No", true: "Yes" } },
      { name: "harassment", type: "noul", instructions: "Is this text targeting or harassing a person or group?", outcomes: { false: "No", true: "Yes" } },
      { name: "spam", type: "noul", instructions: "Is this text unsolicited promotion or a scam?", outcomes: { false: "No", true: "Yes" } },
    ],
  },
  {
    id: "content-tagging",
    label: "Content tagging",
    summary: "Independent topic tags on a JSON document",
    state: {
      title: "Blank screen after clicking pay",
      body: "My checkout page shows a blank screen after I click Pay and no customer can pay.",
    },
    questions: [
      { name: "bug", type: "noul", instructions: "Is this about a defect or an outage?", outcomes: { false: "No", true: "Yes" } },
      { name: "billing", type: "noul", instructions: "Is this about payments, invoices or refunds?", outcomes: { false: "No", true: "Yes" } },
      { name: "sales", type: "noul", instructions: "Is this about pricing, purchasing or a commercial enquiry?", outcomes: { false: "No", true: "Yes" } },
      { name: "needs_reply", type: "noul", instructions: "Does this expect a human response?", outcomes: { false: "No", true: "Yes" } },
    ],
  },
  {
    id: "sentiment",
    label: "Sentiment",
    summary: "Tone on a fixed scale, with a threshold in the summary",
    state: "The new dashboard is genuinely great, but the export still times out every time.",
    questions: [
      {
        name: "sentiment",
        type: "score",
        instructions: "How positive is this text? 0 is very negative, 2 is very positive.",
        scale: ["Negative", "Neutral or mixed", "Positive"],
      },
    ],
  },
  {
    id: "mixed",
    label: "Mixed types",
    summary: "One of each question type, the way the docs do it",
    state: "charged twice for the same order, please refund the extra charge today",
    questions: [
      {
        name: "category",
        type: "choice",
        instructions: "Which team owns this?",
        options_list: [
          { key: "payments", text: "Payments" },
          { key: "product", text: "The product is broken" },
        ],
      },
      { name: "refund", type: "noul", instructions: "Is a refund requested?", outcomes: { false: "No", true: "Yes" } },
      { name: "urgency", type: "score", instructions: "How urgent is this?", scale: ["Whenever", "This week", "Today"] },
    ],
  },
]

export const blankTemplate = templates[0]

export function requestFromTemplate(template: Template): DecideRequest {
  return { state: structuredClone(template.state), questions: structuredClone(template.questions) }
}

export function findTemplate(id: string): Template | undefined {
  return templates.find((template) => template.id === id)
}