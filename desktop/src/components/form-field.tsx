import { Label } from "./ui/label";

export function FormField({
  label,
  htmlFor,
  children,
}: {
  label: string;
  htmlFor: string;
  children: React.ReactNode;
}) {
  return (
    <div>
      <Label htmlFor={htmlFor} className="mb-1">
        {label}
      </Label>
      {children}
    </div>
  );
}
