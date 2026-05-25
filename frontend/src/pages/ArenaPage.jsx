import {ArenaTrack} from "../../wailsjs/go/desktop/API";
import {pageMeta} from "../lib/iscc";
import {TrackPage} from "./TrackPage";

export function ArenaPage() {
  return <TrackPage meta={pageMeta.arena} loader={ArenaTrack} trackKey="arena" />;
}
