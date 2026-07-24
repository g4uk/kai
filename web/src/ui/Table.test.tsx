import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";
import {
  Table,
  TableHead,
  TableBody,
  TableRow,
  TableHeaderCell,
  TableCell,
} from "./Table";

// Step 4 (plan.md): shared Table primitives — same query shape
// JobListPage.test.tsx already uses (getAllByRole("row")), just with
// consistent Tailwind classes added (criterion 4).

describe("Table", () => {
  it('renders table/row/columnheader/cell roles, matching JobListPage\'s getAllByRole("row") usage', () => {
    render(
      <Table>
        <TableHead>
          <TableRow>
            <TableHeaderCell>YouTube URL</TableHeaderCell>
            <TableHeaderCell>Status</TableHeaderCell>
          </TableRow>
        </TableHead>
        <TableBody>
          <TableRow>
            <TableCell>https://youtu.be/abc123</TableCell>
            <TableCell>done</TableCell>
          </TableRow>
          <TableRow>
            <TableCell>https://youtu.be/xyz789</TableCell>
            <TableCell>pending</TableCell>
          </TableRow>
        </TableBody>
      </Table>,
    );

    expect(screen.getByRole("table")).toBeInTheDocument();
    // 1 header row + 2 body rows, mirroring JobListPage.test.tsx's
    // "+1 header row" convention.
    expect(screen.getAllByRole("row")).toHaveLength(3);
    expect(screen.getAllByRole("columnheader")).toHaveLength(2);
    expect(screen.getAllByRole("cell")).toHaveLength(4);
    expect(screen.getByText("https://youtu.be/abc123")).toBeInTheDocument();
  });
});
