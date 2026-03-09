import { useEffect, useState } from "react";
import * as api from "@/api/client";
import type { ServiceInfo } from "@/types";

interface ServiceSlotWidgetProps {
  slot: string;
  prefix: string;
  required: boolean;
  value: string;
  onChange: (value: string) => void;
}

export function ServiceSlotWidget({
  slot,
  prefix,
  required,
  value,
  onChange,
}: ServiceSlotWidgetProps) {
  const [services, setServices] = useState<ServiceInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .listServices()
      .then((all) => {
        const filtered = all.filter((s) => s.prefix === prefix);
        setServices(filtered);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, [prefix]);

  const showError = required && !value;

  return (
    <div className="mb-3">
      <label className="text-sm font-medium text-gray-700">
        {slot}
        {required && <span className="text-red-500 ml-0.5">*</span>}
        <span className="text-xs text-gray-400 ml-1">({prefix}.*)</span>
      </label>
      {loading ? (
        <div className="text-xs text-gray-400 mt-1">Loading services...</div>
      ) : (
        <select
          value={value}
          onChange={(e) => onChange(e.target.value)}
          className={`mt-1 w-full px-3 py-1.5 text-sm border rounded focus:outline-none focus:ring-2 focus:ring-blue-400 ${
            showError ? "border-red-300 bg-red-50" : "border-gray-300"
          }`}
        >
          <option value="">
            {services.length === 0
              ? `No ${prefix}.* services configured`
              : "Select a service..."}
          </option>
          {services.map((svc) => (
            <option key={svc.name} value={svc.name}>
              {svc.name}
              {svc.health === "unhealthy" ? " (unhealthy)" : ""}
            </option>
          ))}
        </select>
      )}
      {showError && (
        <p className="text-xs text-red-500 mt-0.5">
          Required service slot &quot;{slot}&quot; is empty.
        </p>
      )}
    </div>
  );
}
