import { useEffect, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { listFavorites, onFavoritesChanged } from "../api/client";

export function useFavoriteIds(): Set<string> {
  const queryClient = useQueryClient();
  const { data: favorites = [] } = useQuery({
    queryKey: ["favorites"],
    queryFn: listFavorites,
    staleTime: 5 * 60 * 1000,
  });

  useEffect(() => {
    const off = onFavoritesChanged(() => {
      queryClient.invalidateQueries({ queryKey: ["favorites"] });
    });
    return off;
  }, [queryClient]);

  return useMemo(() => new Set(favorites.map((track) => track.id)), [favorites]);
}
