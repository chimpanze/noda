export function Field({
  label,
  children,
  errors,
}: {
  label: string;
  children: React.ReactNode;
  errors?: string[];
}) {
  return (
    <div>
      <label className="text-xs font-medium text-gray-400 uppercase block mb-1">
        {label}
      </label>
      {children}
      {errors &&
        errors.length > 0 &&
        errors.map((err, i) => (
          <p key={i} className="text-xs text-red-500 mt-1">
            {err}
          </p>
        ))}
    </div>
  );
}
