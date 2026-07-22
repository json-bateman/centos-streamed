import { attribute, effect } from "https://cdn.jsdelivr.net/gh/starfederation/datastar@1.0.0-RC.8/bundles/datastar.js";

attribute({
  name: 'theme-storage',
  returnsValue: true,
  requirement: { key: 'denied', value: 'must' },
  apply({ rx }) {
    return effect(() => {
      const str = String(rx?.())

      localStorage.setItem("theme", str)
    });
  },
});
