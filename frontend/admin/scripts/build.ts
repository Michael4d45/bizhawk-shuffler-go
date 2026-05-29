import { buildAdminUI } from "./build-shared.js";

const outDir = await buildAdminUI({ minify: true, clean: true });
console.log(`admin-ui built -> ${outDir}`);
