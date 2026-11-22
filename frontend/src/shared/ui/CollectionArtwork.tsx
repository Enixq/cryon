import { useId } from "react";

const palettes: Record<string, [string, string, string]> = {
  rock: ["#38101e", "#ed4565", "#ffc4a1"],
  pop: ["#321336", "#df59c8", "#ffd8eb"],
  "hip-hop": ["#301907", "#f27d25", "#ffe5a8"],
  electronic: ["#082a35", "#22c4e5", "#b5f9ff"],
  alternative: ["#211238", "#9770ed", "#e4d6ff"],
  jazz: ["#312109", "#d49a36", "#fff0bd"],
  classical: ["#172333", "#8dabc5", "#f2eee4"],
  rnb: ["#06372d", "#28c89c", "#c7ffdf"],
  dance: ["#35112c", "#ef50b1", "#fbd3ff"],
  metal: ["#25101b", "#d54863", "#fdd6bd"],
  "russia-chart": ["#32210a", "#e6a32c", "#fff0ce"],
  "world-chart": ["#052935", "#32c0d8", "#cffaff"],
};

/** Original vector sleeves: each collection has a musical image, rather than a folder or album cover. */
export function CollectionArtwork({ kind, title }: { kind: string; title: string }) {
  const id = useId().replace(/:/g, "");
  const [dark, accent, light] = palettes[kind] || palettes.alternative;
  const ink = {
    stroke: light,
    strokeWidth: 5,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
  };
  const grooves = (
    <g fill="none" stroke={light}>
      {[72, 58, 43, 28].map((r) => (
        <circle key={r} cx="200" cy="192" r={r} opacity=".3" />
      ))}
      <circle cx="200" cy="192" r="13" fill={accent} />
    </g>
  );
  let drawing;
  switch (kind) {
    case "russia-chart":
      drawing = (
        <>
          <text x="28" y="170" fill={light} opacity=".09" fontSize="140" fontWeight="900">
            RU
          </text>
          <g fill={accent} fillOpacity=".2" {...ink} strokeWidth="3">
            <path d="M45 274V221H82V184H109V274M292 274V205H320V155H343V274" />
            <path d="M128 274V211H149V177L163 163L176 177V211H194V274M208 274V201H231V156L244 135L257 156V201H279V274" />
            <path d="M163 164V137M244 135V105M237 116H251M194 274V233H208V274" />
          </g>
          <path
            d="M36 293Q200 330 364 293M55 304Q200 337 345 304"
            fill="none"
            stroke={accent}
            strokeWidth="3"
          />
          <circle cx="291" cy="97" r="24" fill={accent} opacity=".7" />
        </>
      );
      break;
    case "world-chart":
      drawing = (
        <>
          <circle cx="200" cy="193" r="96" fill={`url(#${id}-orb)`} />
          <g fill="none" {...ink} strokeWidth="2.5" opacity=".8">
            <circle cx="200" cy="193" r="96" />
            <ellipse cx="200" cy="193" rx="43" ry="96" />
            <ellipse cx="200" cy="193" rx="96" ry="35" />
            <path d="M104 193H296M200 97V289" />
          </g>
          <ellipse
            cx="200"
            cy="193"
            rx="151"
            ry="46"
            transform="rotate(-28 200 193)"
            fill="none"
            stroke={accent}
            strokeWidth="5"
          />
          <circle cx="326" cy="123" r="9" fill={light} />
          <path d="M69 95H101M85 79V111M320 285H342M331 274V296" {...ink} strokeWidth="2" />
        </>
      );
      break;
    case "rock":
      drawing = (
        <>
          <path
            d="M57 256L152 84M84 298L241 56M217 308L338 111"
            stroke={accent}
            strokeWidth="15"
            opacity=".25"
          />
          <g transform="rotate(29 200 195)" {...ink}>
            <path d="M183 205V92H216V205" fill={dark} />
            <path d="M179 88L175 56H222L219 88Z" fill={accent} />
            <path
              d="M174 183C139 159 114 193 146 217C109 248 132 296 199 301C267 296 292 248 254 217C286 192 259 159 225 183L216 212H183Z"
              fill={`url(#${id}-orb)`}
            />
            <path
              d="M197 75V278M206 75V278M183 113H216M183 135H216M183 158H216M175 256H226"
              strokeWidth="2"
            />
            <rect x="179" y="224" width="41" height="14" rx="3" fill={light} />
          </g>
        </>
      );
      break;
    case "pop":
      drawing = (
        <>
          <path
            d="M205 52L237 126L319 127L257 180L278 258L205 215L133 258L151 180L89 127L172 126Z"
            fill={accent}
            opacity=".27"
          />
          <g transform="rotate(-20 202 192)" {...ink}>
            <rect x="174" y="96" width="62" height="117" rx="31" fill={`url(#${id}-orb)`} />
            <path
              d="M158 171V185Q158 234 205 234Q251 234 251 185V171M205 235V282M177 283H233M184 117H225M184 135H225M184 153H225"
              fill="none"
              strokeWidth="4"
            />
          </g>
          <circle cx="101" cy="266" r="8" fill={light} />
          <circle cx="313" cy="87" r="6" fill={light} />
        </>
      );
      break;
    case "hip-hop":
      drawing = (
        <>
          <path
            d="M41 237L93 90L150 145L241 75L343 140L314 291"
            fill="none"
            stroke={accent}
            strokeWidth="16"
            opacity=".25"
          />
          <g transform="rotate(-8 200 204)" {...ink}>
            <rect x="74" y="144" width="252" height="129" rx="17" fill={dark} />
            <path d="M145 143V112H255V143M83 124L302 91" fill="none" />
            <circle cx="133" cy="214" r="40" fill={`url(#${id}-orb)`} />
            <circle cx="269" cy="214" r="40" fill={`url(#${id}-orb)`} />
            <circle cx="133" cy="214" r="18" fill={dark} />
            <circle cx="269" cy="214" r="18" fill={dark} />
            <path d="M186 179H216M186 193H216M193 218V247M209 218V247" strokeWidth="3" />
          </g>
        </>
      );
      break;
    case "electronic":
      drawing = (
        <>
          <g fill="none" stroke={accent} opacity=".35">
            {[90, 130, 170, 210, 250, 290].map((y) => (
              <path key={y} d={`M48 ${y}H352`} />
            ))}
            {[80, 140, 200, 260, 320].map((x) => (
              <path key={x} d={`M${x} 67V310`} />
            ))}
          </g>
          <path
            d="M42 197H83L102 138L132 260L166 91L201 287L239 128L267 238L290 170L308 197H359"
            fill="none"
            {...ink}
            strokeWidth="7"
          />
          <rect
            x="77"
            y="74"
            width="247"
            height="239"
            rx="24"
            fill="none"
            stroke={accent}
            strokeWidth="3"
          />
          <circle cx="99" cy="92" r="4" fill={light} />
        </>
      );
      break;
    case "jazz":
      drawing = (
        <>
          <circle cx="176" cy="187" r="105" fill={accent} opacity=".15" />
          <g {...ink}>
            <path
              d="M223 84Q271 69 272 107L228 112L201 213Q183 277 252 289Q312 296 325 236L297 173L271 188L287 234Q283 260 254 251Q230 244 239 218L269 132"
              fill={`url(#${id}-orb)`}
            />
            <path d="M290 178L320 164L344 202L316 219Z" fill={accent} />
            <path d="M227 84L213 60H181" fill="none" />
            {[139, 161, 183, 205].map((y) => (
              <circle key={y} cx={251 - (y - 139) / 4} cy={y} r="5" fill={light} strokeWidth="2" />
            ))}
          </g>
          <path
            d="M92 158V103L118 97V151M86 166Q99 174 99 159M113 157Q126 163 126 149"
            fill="none"
            {...ink}
            strokeWidth="3"
          />
        </>
      );
      break;
    case "classical":
      drawing = (
        <>
          <g {...ink}>
            <path d="M90 227V139Q149 50 267 82Q300 91 291 152L247 225Z" fill={`url(#${id}-orb)`} />
            <path d="M94 227H290V269H94ZM111 270L102 307M273 269L284 307" fill={dark} />
            {Array.from({ length: 14 }, (_, i) => (
              <path key={i} d={`M${107 + i * 13} 228V268`} strokeWidth="1.5" />
            ))}
            {[0, 1, 3, 4, 5, 7, 8, 10, 11, 12].map((i) => (
              <rect
                key={i}
                x={108 + i * 13}
                y="228"
                width="8"
                height="24"
                fill={light}
                stroke="none"
              />
            ))}
            <path d="M107 147Q153 80 270 96" fill="none" opacity=".5" strokeWidth="2" />
          </g>
        </>
      );
      break;
    case "dance":
      drawing = (
        <>
          <path
            d="M200 49V100M47 286L164 130M353 286L236 130M74 307L177 135M326 307L223 135"
            fill="none"
            stroke={accent}
            strokeWidth="4"
            opacity=".6"
          />
          <circle cx="200" cy="166" r="76" fill={`url(#${id}-orb)`} />
          <g fill="none" stroke={light} strokeWidth="2">
            <circle cx="200" cy="166" r="76" />
            {[-48, -24, 0, 24, 48].map((d) => (
              <ellipse
                key={d}
                cx="200"
                cy="166"
                rx={Math.sqrt(76 * 76 - d * d)}
                ry={Math.max(12, 76 - Math.abs(d))}
                opacity=".55"
              />
            ))}
            <path d="M133 135H267M124 166H276M133 197H267M200 90V242" />
          </g>
          <path d="M73 102H97M85 90V114M296 267H328M312 251V283" {...ink} strokeWidth="3" />
        </>
      );
      break;
    case "metal":
      drawing = (
        <>
          <path
            d="M67 211L123 83L180 159L228 64L271 159L329 101L317 267L246 307L156 292Z"
            fill={accent}
            opacity=".2"
          />
          <path d="M229 69L124 215H187L163 315L286 169H220Z" fill={`url(#${id}-orb)`} {...ink} />
          <path d="M92 275L116 243M294 296L306 259M126 80L151 116" {...ink} strokeWidth="3" />
        </>
      );
      break;
    case "rnb":
      drawing = (
        <>
          <g fill="none" stroke={accent} strokeWidth="4" opacity=".6">
            {[0, 22, 44, 66].map((d) => (
              <path
                key={d}
                d={`M-20 ${160 + d}C81 ${37 + d} 160 ${312 + d} 265 ${162 + d}S367 ${72 + d} 429 ${181 + d}`}
              />
            ))}
          </g>
          <circle cx="200" cy="192" r="83" fill={dark} fillOpacity=".8" />
          {grooves}
          <path
            d="M182 185Q168 166 182 156Q198 145 207 163Q225 146 235 160Q244 176 207 203Z"
            fill={light}
            transform="translate(-7 17)"
          />
        </>
      );
      break;
    default:
      drawing = (
        <>
          <circle cx="265" cy="105" r="35" fill={light} opacity=".8" />
          <circle cx="251" cy="94" r="35" fill={dark} />
          <path d="M26 286L128 121L228 281L288 186L374 302" fill={accent} opacity=".3" />
          <path d="M33 288L129 139L171 204L209 176L330 289" fill="none" {...ink} strokeWidth="3" />
          {grooves}
          <path d="M76 92H92M84 84V100M312 148H326M319 141V155" {...ink} strokeWidth="2" />
        </>
      );
  }
  return (
    <svg
      viewBox="0 0 400 400"
      className="aspect-square w-full transition-transform duration-500 group-hover:scale-[1.035]"
      role="img"
      aria-label={title}
    >
      <defs>
        <linearGradient id={`${id}-base`} x2="1" y2="1">
          <stop stopColor={dark} />
          <stop offset="1" stopColor="#080c16" />
        </linearGradient>
        <radialGradient id={`${id}-orb`} cx="30%" cy="20%" r="95%">
          <stop stopColor={accent} stopOpacity=".85" />
          <stop offset="1" stopColor={dark} />
        </radialGradient>
        <radialGradient id={`${id}-glow`}>
          <stop stopColor={accent} stopOpacity=".3" />
          <stop offset="1" stopColor={accent} stopOpacity="0" />
        </radialGradient>
      </defs>
      <rect width="400" height="400" fill={`url(#${id}-base)`} />
      <circle cx="200" cy="165" r="219" fill={`url(#${id}-glow)`} />
      <rect
        x="15"
        y="15"
        width="370"
        height="370"
        rx="18"
        fill="none"
        stroke={light}
        strokeOpacity=".13"
      />
      <text
        x="31"
        y="44"
        fill={light}
        opacity=".55"
        fontSize="10"
        fontWeight="700"
        letterSpacing="4"
      >
        CRYON
      </text>
      <path d="M333 35H367M333 42H354" stroke={light} strokeOpacity=".45" strokeWidth="2" />
      {drawing}
    </svg>
  );
}
