import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import sharp from "sharp";
const here = dirname(fileURLToPath(import.meta.url));
const svg = readFileSync(join(here, "appicon.svg"), "utf8");
const render = (source, size) =>
  sharp(Buffer.from(source)).resize(size, size).png().toBuffer();
writeFileSync(join(here, "appicon.png"), await render(svg, 1024));
writeFileSync(
  join(here, "..", "frontend", "public", "favicon.png"),
  await render(svg, 64),
);
const sizes = [16, 24, 32, 48, 64, 128, 256];
const pngs = await Promise.all(sizes.map((size) => render(svg, size)));
const header = Buffer.alloc(6 + sizes.length * 16);
header.writeUInt16LE(1, 2);
header.writeUInt16LE(sizes.length, 4);
let offset = header.length;
sizes.forEach((size, index) => {
  const at = 6 + index * 16;
  header[at] = header[at + 1] = size === 256 ? 0 : size;
  header.writeUInt16LE(1, at + 4);
  header.writeUInt16LE(32, at + 6);
  header.writeUInt32LE(pngs[index].length, at + 8);
  header.writeUInt32LE(offset, at + 12);
  offset += pngs[index].length;
});
writeFileSync(
  join(here, "windows", "icon.ico"),
  Buffer.concat([header, ...pngs]),
);
const foreground = svg
  .replace(/<rect[^>]+\/>/, "")
  .replace('viewBox="0 0 512 512"', 'viewBox="-164 -164 840 840"');
for (const [density, size] of [
  ["mdpi", 48],
  ["hdpi", 72],
  ["xhdpi", 96],
  ["xxhdpi", 144],
  ["xxxhdpi", 192],
]) {
  const folder = join(
    here,
    "..",
    "android",
    "app",
    "src",
    "main",
    "res",
    `mipmap-${density}`,
  );
  mkdirSync(folder, { recursive: true });
  writeFileSync(join(folder, "ic_launcher.png"), await render(svg, size));
  writeFileSync(join(folder, "ic_launcher_round.png"), await render(svg, size));
  writeFileSync(
    join(folder, "ic_launcher_foreground.png"),
    await render(foreground, Math.round((size * 108) / 48)),
  );
}
console.log("Updated Windows, Android adaptive and PNG icons.");
