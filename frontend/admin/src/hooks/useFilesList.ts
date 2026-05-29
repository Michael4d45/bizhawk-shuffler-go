import { useEffect, useState } from "react";
import { fetchFilesList } from "../api.js";

export function useFilesList(autoLoad = false) {
  const [files, setFiles] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);

  async function refresh() {
    setLoading(true);
    try {
      setFiles(await fetchFilesList());
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    if (autoLoad) void refresh();
  }, [autoLoad]);

  return { files, loading, refresh };
}
