"use client";

import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

import { getDefaultRouteForRole, getStoredAuthSession } from "@/store/auth";

export default function HomePage() {
  const navigate = useNavigate();

  useEffect(() => {
    let active = true;

    const redirect = async () => {
      const session = await getStoredAuthSession();
      if (!active) {
        return;
      }
      navigate(session ? getDefaultRouteForRole(session.role) : "/login", { replace: true });
    };

    void redirect();
    return () => {
      active = false;
    };
  }, [navigate]);

  return null;
}
