import { describe, it, expect } from "vitest";
import { isValidYoutubeUrl } from "./youtubeUrl";

// Step 6 (plan.md): pure client-side YouTube URL validator, approximating
// internal/handler/jobs.go's isValidYoutubeURL closely enough to reject
// obvious garbage (server remains authoritative — criterion 9).

describe("isValidYoutubeUrl", () => {
  const cases: Array<{ name: string; input: string; want: boolean }> = [
    {
      name: "a standard watch URL",
      input: "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
      want: true,
    },
    {
      name: "a watch URL with extra query params",
      input: "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=30s&list=abc",
      want: true,
    },
    {
      name: "a bare youtube.com watch URL without www",
      input: "https://youtube.com/watch?v=dQw4w9WgXcQ",
      want: true,
    },
    {
      name: "a youtu.be short link",
      input: "https://youtu.be/dQw4w9WgXcQ",
      want: true,
    },
    {
      name: "a youtu.be short link with a trailing slash",
      input: "https://youtu.be/dQw4w9WgXcQ/",
      want: true,
    },
    {
      name: "a youtube.com shorts URL",
      input: "https://www.youtube.com/shorts/dQw4w9WgXcQ",
      want: true,
    },
    {
      name: "an empty string",
      input: "",
      want: false,
    },
    {
      name: "a non-YouTube host",
      input: "https://vimeo.com/12345",
      want: false,
    },
    {
      name: "a malformed URL with no scheme",
      input: "not a url at all",
      want: false,
    },
    {
      name: "a youtube.com watch URL missing the v param",
      input: "https://www.youtube.com/watch",
      want: false,
    },
    {
      name: "a youtube.com root URL with no path",
      input: "https://www.youtube.com/",
      want: false,
    },
  ];

  for (const tc of cases) {
    it(`returns ${tc.want} for ${tc.name}`, () => {
      expect(isValidYoutubeUrl(tc.input)).toBe(tc.want);
    });
  }
});
