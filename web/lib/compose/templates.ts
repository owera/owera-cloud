// Seed templates surfaced on /jobs when the user has no saved templates yet.
//
// Each template is a deep-link into /compose with prefilled archetype, level,
// and a starter prompt. The composer parses these from search params on SSR.

import type { TemplateSeed } from "@/components/compose/template-card";

export const STARTER_TEMPLATES: ReadonlyArray<TemplateSeed> = [
  {
    id: "daily-ai-brief",
    archetype: "brief",
    name: "Daily AI brief",
    tagline: "What shipped in AI yesterday, in 300 words.",
    cadence: "DAILY · 09:00",
    href:
      "/compose?archetype=brief&level=standard&prompt=" +
      encodeURIComponent(
        "Yesterday's most significant AI news, papers, and open-source releases, in a tight 300-word brief.",
      ),
  },
  {
    id: "competitive-teardown",
    archetype: "research",
    name: "Competitive teardown",
    tagline: "A research memo with citations on a topic you choose.",
    cadence: "AD-HOC",
    href:
      "/compose?archetype=research&level=advanced&prompt=" +
      encodeURIComponent(
        "Competitive teardown of three vector DBs: pricing, throughput, operational story. 300-word memo with citations.",
      ),
  },
  {
    id: "inbox-triage",
    archetype: "triage",
    name: "Inbox triage",
    tagline: "Classify and route incoming items by your rules.",
    cadence: "DAILY · 09:00",
    href:
      "/compose?archetype=triage&level=standard&prompt=" +
      encodeURIComponent(
        "Triage incoming support email: urgent if production-down or billing-blocked, otherwise route by product area.",
      ),
  },
  {
    id: "watch-mentions",
    archetype: "watch",
    name: "Watch for mentions",
    tagline: "Alert me when a source mentions our product or a competitor.",
    cadence: "HOURLY",
    href:
      "/compose?archetype=watch&level=standard&prompt=" +
      encodeURIComponent(
        "Watch Hacker News and Reddit /r/MachineLearning for posts that mention Owera or a competitor by name.",
      ),
  },
  {
    id: "build-with-tests",
    archetype: "build",
    name: "Build with tests",
    tagline: "Generate code with an eval pass — the result actually runs.",
    cadence: "AD-HOC",
    href:
      "/compose?archetype=build&level=expert&prompt=" +
      encodeURIComponent(
        "TypeScript function that deduplicates an array of objects by a key. Include 5 unit tests; run them.",
      ),
  },
  {
    id: "custom",
    archetype: "custom",
    name: "Start from scratch",
    tagline: "Open prompt with every knob exposed.",
    cadence: "AD-HOC",
    href: "/compose?archetype=custom&level=advanced",
  },
];
