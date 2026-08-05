import { createRoot } from "react-dom/client";
import Chart from "chart.js/auto";
import App from "./App";

// The Genspark reference loads Chart.js from a <head> script. The React shell
// deliberately copies only styles from that head, so expose the bundled build
// before mounting the reference scripts. Charts then work without CDN access.
window.Chart = Chart;

createRoot(document.getElementById("root")).render(<App />);
