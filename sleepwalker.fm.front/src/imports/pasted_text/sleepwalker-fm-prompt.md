🎯 MASTER PROMPT — sleepwalker.fm (DATA-DRIVEN UI ONLY)

Design a premium frontend UI/UX for a web application called sleepwalker.fm — a high-end Spotify listening analytics platform.

The product visual style is a dark, futuristic, luxury SaaS interface focused on music data exploration.

⸻

⚠️ CRITICAL CONSTRAINT (NON-NEGOTIABLE)

The entire interface MUST be built strictly on real backend data from the API endpoints listed below.

You are NOT allowed to:

* Invent new data types
* Add AI personality, mood inference, psychological profiling
* Fabricate listening history beyond provided endpoints
* Display metrics that cannot be derived from the API
* Assume Spotify provides features it does not

You ARE allowed to:

* Visualize real API data
* Compute derived metrics ONLY when logically possible from provided data
* Gracefully handle missing or limited data (warnings, fallback states)

⸻

🔌 BACKEND DATA SOURCES (TRUTH LAYER)

🎧 Core Spotify Data

* GET /api/spotify/top/artists/:userId
* GET /api/spotify/top/tracks/:userId
* GET /api/spotify/recently-played/:userId
* GET /api/spotify/audio-features/:userId (may return warning due to Spotify limitations / 403)

⸻

📊 WRAPPED SYSTEM

* GET /api/wrapped/summary/:userId
* GET /api/wrapped/insights/:userId
* GET /api/wrapped/timeline/:userId
* GET /api/wrapped/compare/:userId

⸻

📈 STATS SYSTEM

* GET /api/stats/profile/:userId
* GET /api/stats/genres/:userId
* GET /api/stats/listening-time/:userId

⸻

🎯 RECOMMENDATIONS SYSTEM

* GET /api/recommendations/:userId
* POST /api/recommendations/playlist/:userId

⸻

⚠️ FALLBACK & EDGE CASE RULES

* audio-features may return a warning due to Spotify API limitations → UI must degrade gracefully
* recommendations may fallback to top_tracks seeds → must be clearly labeled as fallback source
* playlist creation may return fallback URIs → UI must offer export option instead of save
* all warnings must be subtle UI indicators, NOT error screens

⸻

🎨 VISUAL DIRECTION (sleepwalker.fm IDENTITY)

Style:

* premium dark SaaS aesthetic
* futuristic cold purple palette
* neon violet + indigo accents
* graphite black backgrounds
* soft gradient lighting
* subtle glassmorphism (not overdone)

⸻

Design references:

* Linear (layout clarity)
* Spotify Wrapped (data storytelling)
* Arc Browser (motion feel)
* Vercel (clean SaaS polish)
* Apple Music motion design

⸻

Design principles:

* data-first interface
* every visual element must map to real or derived API data
* no fake storytelling layers
* emotionally immersive but grounded in real metrics
* minimal clutter, high hierarchy clarity

⸻

📊 APPLICATION STRUCTURE

1. DASHBOARD (MAIN VIEW)

Uses:

* top artists
* top tracks
* recently played
* stats/profile
* genres
* listening time

UI components:

* Top Artists horizontal carousel
* Top Tracks ranked list
* Recently Played timeline feed
* Listening time summary card
* Genre distribution chart
* Audio features visualization (only if available)

⸻

2. WRAPPED SUMMARY PAGE

Uses:

* /wrapped/summary
* /wrapped/insights

UI:

* top artist highlight
* top track highlight
* top genre
* listener tag (computed backend insight only)
* total listening minutes
* insight cards

⚠️ No AI-generated personality or emotional profiling allowed.

⸻

3. LISTENING TIMELINE PAGE

Uses:

* /wrapped/timeline
* /recently-played

UI:

* hourly heatmap
* daily activity chart
* scrollable listening history feed

⸻

4. COMPARE PAGE

Uses:

* /wrapped/compare

UI:

* overlap vs new tracks visualization
* shared artists graph
* listening similarity score (if provided by backend)

⸻

5. GENRES & PROFILE PAGE

Uses:

* /stats/genres
* /stats/profile

UI:

* genre distribution chart
* audio feature averages (energy, valence, tempo, etc.)
* listening profile summary cards

⸻

6. RECOMMENDATIONS PAGE

Uses:

* /recommendations

UI:

* recommended tracks list
* source label (top_tracks or seed-based fallback)
* fallback indicator when recommendation engine is degraded

⸻

7. PLAYLIST EXPORT PAGE

Uses:

* POST /recommendations/playlist

UI:

* generated playlist preview
* success state with playlist ID or URL
* fallback state showing exportable track URIs

⸻

📈 DATA VISUALIZATION RULES

Allowed charts only:

* bar charts (top artists/tracks)
* radial charts (audio features)
* heatmaps (listening activity)
* line charts (time-based listening trends)
* distribution charts (genres)

All charts MUST be derived from real API data or computed backend responses.

⸻

⚡ MOTION & INTERACTION DESIGN

* Framer Motion-style smooth transitions
* staggered card animations on load
* hover glow effects (subtle, premium)
* animated chart rendering
* parallax background depth movement
* soft gradient ambient motion
* glass panel floating effect

⸻

🧩 COMPONENT SYSTEM

Reusable UI components:

* stat cards (data-bound)
* chart containers
* artist cards
* track list rows
* timeline blocks
* recommendation cards
* warning badges (non-intrusive)
* fallback state components

⸻

🚨 EMPTY / LOADING / WARNING STATES

Every data source must support:

* loading state (skeleton UI)
* empty state (no data available)
* warning state (Spotify limitations or fallback mode)

Examples:

* audio-features unavailable → “Limited analysis available”
* recommendations fallback → “Based on top tracks instead”
* missing timeline → graceful placeholder visualization

⸻

🧠 UX GOAL

sleepwalker.fm must feel like:

a premium, real-time Spotify listening intelligence platform

NOT:

* AI personality generator
* emotional profiling tool
* fantasy music psychology app

⸻

🎯 FINAL OUTPUT REQUIREMENTS

Generate a complete multi-page UI system that includes:

* dashboard
* wrapped experience
* analytics pages
* recommendation system UI
* playlist export flow

Must be:

* fully responsive (desktop-first, mobile-adaptive)
* visually consistent design system
* production-ready UI structure
* strictly aligned with provided API endpoints
* premium dark purple cinematic aesthetic

⸻

🚀 END GOAL

A realistic, implementable frontend design that can be directly translated into a Next.js + React application without changing data assumptions or backend constraints.