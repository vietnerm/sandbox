import { Ban } from "lucide-react";
import { Panel } from "./primitives";

/** shown when a deep link points at a view minictd does not provide */
export function UnsupportedView({ what }: { what: string }) {
  return (
    <Panel className="mx-auto mt-10 max-w-xl p-8 text-center">
      <Ban className="mx-auto mb-3 size-8 text-gray-500" aria-hidden />
      <h1 className="text-lg font-semibold text-ink">{what}: không có trên minictd</h1>
      <p className="mt-2 text-sm text-gray-400">
        minictd là control plane một người dùng, một máy (manifest §4), nên trang này không áp dụng.
        <br />
        <span className="text-gray-500">Not available on minictd (single-user, single-node).</span>
      </p>
    </Panel>
  );
}
