import {PracticeTrack} from "../../wailsjs/go/desktop/API";
import {pageMeta} from "../lib/iscc";
import {TrackPage} from "./TrackPage";

export function PracticePage() {
  return <TrackPage meta={pageMeta.practice} loader={PracticeTrack} trackKey="practice" />;
}
